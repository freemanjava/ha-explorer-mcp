package analysis

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

// MeshKind names which mesh metric an entity carries.
type MeshKind string

const (
	MeshLinkQuality    MeshKind = "link_quality"
	MeshSignalStrength MeshKind = "signal_strength"
)

// signalStrengthDeviceClass is the semantic marker RSSI carries on both ZHA and
// Zigbee2MQTT; LQI deliberately carries none (D-05-5), so it is found by name.
const signalStrengthDeviceClass = "signal_strength"

// Name hints for the metric entities, matched against the entity id's object
// part when no device_class settles it. The table is five strings, not a
// plugin seam, and is read from the entity id — never the registry platform or
// an integration name (D-05-5, rule 6). Evidence for each spelling:
// docs/research/2026-09-05-zigbee-mesh-metric-normalization.md.
var (
	linkQualityHints    = []string{"lqi", "linkquality", "link_quality"}
	signalStrengthHints = []string{"rssi", "signal_strength"}
)

const (
	meshSensorDomain = "sensor"
	// meshMinSamples is the fewest numeric readings that make a min/mean worth
	// reporting; below it ConfidenceFor would call the evidence low anyway, but
	// zero readings must be absence, never a value (rule 7).
	meshMinSamples = 1
)

// MeshMetric is one resolved metric entity of one device.
type MeshMetric struct {
	Device model.DeviceID
	Kind   MeshKind
	Entity model.Entity
}

// MeshResolution is what ResolveMeshMetrics found: the readable metric
// entities, and a MissingEvidence row for each metric that exists but cannot be
// read, or does not exist for a device that otherwise reports mesh health.
type MeshResolution struct {
	Metrics []MeshMetric
	Missing []model.MissingEvidence
}

// ResolveMeshMetrics picks, per device, the link-quality and signal-strength
// entities by device_class first and the hint table second (D-05-5). Entities
// without a device are ignored: a metric is only meaningful per device. The
// caller passes entities the privacy profile already permits. The result is
// ordered by device id then kind, so output is deterministic.
func ResolveMeshMetrics(entities []model.Entity) MeshResolution {
	byDevice := map[model.DeviceID]*deviceMesh{}
	var order []model.DeviceID
	for _, e := range entities {
		kind, ok := meshKindOf(e)
		if !ok || e.DeviceID == "" {
			continue
		}
		d, seen := byDevice[e.DeviceID]
		if !seen {
			d = &deviceMesh{}
			byDevice[e.DeviceID] = d
			order = append(order, e.DeviceID)
		}
		d.add(kind, e)
	}
	slices.Sort(order)

	var res MeshResolution
	for _, id := range order {
		byDevice[id].resolve(id, &res)
	}
	return res
}

// deviceMesh collects one device's candidate entities per kind.
type deviceMesh struct {
	linkQuality    []model.Entity
	signalStrength []model.Entity
}

func (d *deviceMesh) add(kind MeshKind, e model.Entity) {
	if kind == MeshSignalStrength {
		d.signalStrength = append(d.signalStrength, e)
		return
	}
	d.linkQuality = append(d.linkQuality, e)
}

func (d *deviceMesh) resolve(id model.DeviceID, res *MeshResolution) {
	d.resolveKind(id, MeshLinkQuality, d.linkQuality, res)
	if len(d.signalStrength) == 0 {
		// Mesh-shaped siblings but no RSSI: Zigbee2MQTT exposes none (D-05-5).
		if len(d.linkQuality) > 0 {
			res.Missing = append(res.Missing, model.MissingEvidence{
				What: meshWhat(MeshSignalStrength, id), Source: recorderSource, Reason: model.MissingNotExposed,
				Detail: "the device exposes link quality but no signal strength entity",
			})
		}
		return
	}
	d.resolveKind(id, MeshSignalStrength, d.signalStrength, res)
}

// resolveKind prefers the first enabled candidate (entity-id order is the
// caller's). When every candidate is disabled the metric records nothing, and
// that is named rather than reported as a value.
func (d *deviceMesh) resolveKind(id model.DeviceID, kind MeshKind, candidates []model.Entity, res *MeshResolution) {
	if len(candidates) == 0 {
		return
	}
	for _, e := range candidates {
		if e.DisabledBy == "" {
			res.Metrics = append(res.Metrics, MeshMetric{Device: id, Kind: kind, Entity: e})
			return
		}
	}
	res.Missing = append(res.Missing, model.MissingEvidence{
		What: meshWhat(kind, id), Source: recorderSource, Reason: model.MissingEntityDisabled,
		Detail: "the entity carrying this metric is disabled, so it records nothing; enabling it is the fix",
	})
}

func meshWhat(kind MeshKind, id model.DeviceID) string {
	return fmt.Sprintf("%s for device %s", strings.ReplaceAll(string(kind), "_", " "), id)
}

// IsMeshEntity reports whether the entity would resolve as a mesh metric, so a
// caller can apply the privacy profile to metric entities alone.
func IsMeshEntity(e model.Entity) bool {
	_, ok := meshKindOf(e)
	return ok
}

// meshKindOf classifies a sensor entity as a mesh metric, or reports it is
// neither. device_class decides first; the name only when it does not.
func meshKindOf(e model.Entity) (MeshKind, bool) {
	if e.Domain != meshSensorDomain && !strings.HasPrefix(string(e.ID), meshSensorDomain+".") {
		return "", false
	}
	if e.DeviceClass == signalStrengthDeviceClass {
		return MeshSignalStrength, true
	}
	object := string(e.ID)
	if i := strings.IndexByte(object, '.'); i >= 0 {
		object = object[i+1:]
	}
	switch {
	case hasHintSuffix(object, signalStrengthHints):
		return MeshSignalStrength, true
	case hasHintSuffix(object, linkQualityHints):
		return MeshLinkQuality, true
	}
	return "", false
}

// hasHintSuffix matches the hint as the whole object id or its last
// underscore-delimited part, so "bulb_lqi" matches but "equalizer_lqix" and
// "mylqi" do not.
func hasHintSuffix(object string, hints []string) bool {
	for _, h := range hints {
		if object == h || strings.HasSuffix(object, "_"+h) {
			return true
		}
	}
	return false
}

// MeshEvidence turns one metric entity's read history into one Evidence
// (min, mean, samples). Only numeric states count; "unavailable" and "unknown"
// are not readings. With no numeric reading there is no evidence at all —
// never a zero (rule 7) — and ok is false. Coverage is the share of
// [from, to] from the first numeric reading onward, since earlier time is
// unobserved, not bad.
func MeshEvidence(m MeshMetric, from, to time.Time, points []model.HistoryPoint) (ev model.Evidence, ok bool) {
	var (
		samples  int
		sum      float64
		minimum  = math.Inf(1)
		firstAt  time.Time
		windowNs = to.Sub(from)
	)
	for _, p := range points {
		v, err := strconv.ParseFloat(p.State, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if samples == 0 {
			firstAt = p.Timestamp
		}
		samples++
		sum += v
		minimum = math.Min(minimum, v)
	}
	if samples < meshMinSamples || windowNs <= 0 {
		return model.Evidence{}, false
	}
	coverage := float64(to.Sub(firstAt)) / float64(windowNs)
	return model.Evidence{
		ID:          model.EvidenceID(fmt.Sprintf("mesh_%s_%s", m.Kind, m.Entity.ID)),
		Observation: strings.ReplaceAll(string(m.Kind), "_", " ") + " readings of a device's mesh link",
		Source:      recorderSource,
		From:        from,
		To:          to,
		Measurements: map[string]float64{
			"min":     minimum,
			"mean":    sum / float64(samples),
			"samples": float64(samples),
		},
		SampleSize: samples,
		Coverage:   math.Min(1, math.Max(0, coverage)),
	}, true
}
