package policy

import (
	"errors"
	"strings"
	"testing"

	"github.com/freemanjava/ha-explorer-mcp/internal/model"
)

func TestProfile_EmptyConfiguration_DefaultsToMask(t *testing.T) {
	// The PRIVATE-handling decision of 2026-08-25: mask is the default, and
	// it must fall out of empty configuration rather than of a setup step
	// somebody can forget.
	p, err := NewProfile("")
	if err != nil {
		t.Fatalf("NewProfile(\"\"): %v", err)
	}
	if p.Private != HandlingMask {
		t.Errorf("Private = %v, want %v", p.Private, HandlingMask)
	}

	var zero Profile
	if zero.Private != HandlingMask {
		t.Errorf("zero Profile Private = %v, want %v", zero.Private, HandlingMask)
	}
}

func TestNewProfile_KnownNames_Selectable(t *testing.T) {
	for name, want := range map[string]Handling{
		"mask":  HandlingMask,
		"allow": HandlingAllow,
		"deny":  HandlingDeny,
		"MASK":  HandlingMask,
		" deny": HandlingDeny,
	} {
		p, err := NewProfile(name)
		if err != nil {
			t.Fatalf("NewProfile(%q): %v", name, err)
		}
		if p.Private != want {
			t.Errorf("NewProfile(%q).Private = %v, want %v", name, p.Private, want)
		}
	}
}

func TestNewProfile_UnknownName_Rejected(t *testing.T) {
	// Fail closed: an unrecognized profile name is a configuration error, not
	// a silent fallback to the loosest setting.
	if _, err := NewProfile("permissive"); err == nil {
		t.Fatal("NewProfile(\"permissive\") = nil error, want rejection")
	}
}

func TestProfile_Decide_SecretAlwaysRedacted(t *testing.T) {
	// No profile may return a SECRET value. Read-only-ness and secrecy are
	// not configurable (CLAUDE.md, Configuration).
	for _, name := range []string{"mask", "allow", "deny"} {
		p, err := NewProfile(name)
		if err != nil {
			t.Fatalf("NewProfile(%q): %v", name, err)
		}
		if got := p.Decide(SensitivitySecret); got != ActionRedact {
			t.Errorf("profile %q Decide(secret) = %v, want %v", name, got, ActionRedact)
		}
	}
}

func TestProfile_Decide_NormalAlwaysAllowed(t *testing.T) {
	for _, name := range []string{"mask", "allow", "deny"} {
		p, _ := NewProfile(name)
		if got := p.Decide(SensitivityNormal); got != ActionAllow {
			t.Errorf("profile %q Decide(normal) = %v, want %v", name, got, ActionAllow)
		}
	}
}

func TestProfile_Decide_PrivateFollowsProfile(t *testing.T) {
	for name, want := range map[string]Action{
		"mask":  ActionMask,
		"allow": ActionAllow,
		"deny":  ActionDeny,
	} {
		p, _ := NewProfile(name)
		if got := p.Decide(SensitivityPrivate); got != want {
			t.Errorf("profile %q Decide(private) = %v, want %v", name, got, want)
		}
	}
}

func TestProfile_CheckHistoryScope_PrivateDomainSweep_DeniedByDefault(t *testing.T) {
	// Appendix B: bulk history over a PRIVATE domain is refused under the
	// default profile with an explicit policy error. Masking preserves one
	// entity's shape in time; a domain-wide sweep is the occupancy timeline
	// itself, in volume, and masking does not take that back.
	var p Profile // default: mask

	err := p.CheckHistoryScope(HistoryScope{Domains: []string{"device_tracker"}})
	if err == nil {
		t.Fatal("CheckHistoryScope(device_tracker sweep) = nil, want denial")
	}
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("error %v does not match ErrPolicyDenied", err)
	}

	var pe *PolicyError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a *PolicyError", err)
	}
	if pe.Sensitivity != SensitivityPrivate {
		t.Errorf("PolicyError.Sensitivity = %v, want %v", pe.Sensitivity, SensitivityPrivate)
	}
	if pe.Subject != "device_tracker" {
		t.Errorf("PolicyError.Subject = %q, want %q", pe.Subject, "device_tracker")
	}
	// The agent must be able to act on the refusal: it says what was refused
	// and why, and it never carries a value.
	if !strings.Contains(pe.Error(), "device_tracker") || pe.Reason == "" {
		t.Errorf("unhelpful policy error: %q (reason %q)", pe.Error(), pe.Reason)
	}
}

func TestProfile_CheckHistoryScope_NamedPrivateEntities_AllowedUnderMask(t *testing.T) {
	// Naming entities is a targeted diagnostic — a flaky lock or an
	// unreliable presence sensor is exactly what this server exists to
	// investigate — and mask keeps its shape in time while destroying its
	// meaning.
	var p Profile
	if err := p.CheckHistoryScope(HistoryScope{Entities: []model.EntityID{"lock.front_door"}}); err != nil {
		t.Fatalf("CheckHistoryScope(named lock) = %v, want nil", err)
	}
}

func TestProfile_CheckHistoryScope_NamedPrivateEntities_DeniedUnderDeny(t *testing.T) {
	p, _ := NewProfile("deny")
	err := p.CheckHistoryScope(HistoryScope{
		Entities: []model.EntityID{"sensor.temperature", "person.owner"},
	})
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("CheckHistoryScope under deny = %v, want ErrPolicyDenied", err)
	}
	var pe *PolicyError
	if errors.As(err, &pe) && pe.Subject != "person.owner" {
		t.Errorf("PolicyError.Subject = %q, want the offending entity", pe.Subject)
	}
}

func TestProfile_CheckHistoryScope_PrivateDomainSweep_AllowedUnderAllow(t *testing.T) {
	p, _ := NewProfile("allow")
	if err := p.CheckHistoryScope(HistoryScope{Domains: []string{"person"}}); err != nil {
		t.Fatalf("CheckHistoryScope under allow = %v, want nil", err)
	}
}

func TestProfile_CheckHistoryScope_NormalScope_Allowed(t *testing.T) {
	for _, name := range []string{"mask", "allow", "deny"} {
		p, _ := NewProfile(name)
		sc := HistoryScope{
			Entities: []model.EntityID{"sensor.living_room_temperature"},
			Domains:  []string{"sensor", "light"},
		}
		if err := p.CheckHistoryScope(sc); err != nil {
			t.Errorf("profile %q refused a NORMAL scope: %v", name, err)
		}
	}
}

func TestPolicyError_NeverCarriesAValue(t *testing.T) {
	// A denial explains what was refused; it must not leak the very value it
	// refused to return.
	err := (&Profile{Private: HandlingDeny}).CheckHistoryScope(
		HistoryScope{Entities: []model.EntityID{"device_tracker.owner_phone"}})
	if err == nil {
		t.Fatal("want denial")
	}
	if strings.Contains(err.Error(), "not_home") || strings.Contains(err.Error(), "52.3") {
		t.Errorf("policy error carries data: %q", err.Error())
	}
}

func TestCoordinatePrecision_IsOneDecimal(t *testing.T) {
	// The 2026-08-25 decision: coarsen to one decimal (~11 km), keeping
	// sun/weather/timezone correlation while removing address-level
	// identification. internal/redact applies it; policy decides it.
	if CoordinateDecimals != 1 {
		t.Errorf("CoordinateDecimals = %d, want 1", CoordinateDecimals)
	}
}
