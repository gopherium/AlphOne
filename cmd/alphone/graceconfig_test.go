// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/tenant"
)

func TestTheGraceWindowFallsBackToItsDefault(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.machineGrace != tenant.DefaultMachineGrace {
		t.Errorf("machineGrace = %v, want the default %v", held.machineGrace, tenant.DefaultMachineGrace)
	}
}

func TestTheGraceWindowIsReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":         "postgres://localhost/x",
		"ALPHONE_TENANT_MACHINE_GRACE": "720h",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.machineGrace != 720*time.Hour {
		t.Errorf("machineGrace = %v, want 720h", held.machineGrace)
	}
}

func TestTheGraceWindowRefusesAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"not a duration":  "a fortnight",
		"a negative span": "-1h",
	}
	for testName, raw := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL":         "postgres://localhost/x",
				"ALPHONE_TENANT_MACHINE_GRACE": raw,
			}))

			if err == nil {
				t.Errorf("loadRunConfig() with %q error = nil, want the value refused", raw)
			}
		})
	}
}

func TestAZeroGraceWindowStopsMachineTrafficAtOnce(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":         "postgres://localhost/x",
		"ALPHONE_TENANT_MACHINE_GRACE": "0",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	closed := tenant.Tenant{Deactivated: true, DeactivatedAt: time.Now()}
	if closed.AcceptsMachineTraffic(time.Now(), held.machineGrace) {
		t.Error("a zero window still accepts machine traffic, want it stopped at once")
	}
}
