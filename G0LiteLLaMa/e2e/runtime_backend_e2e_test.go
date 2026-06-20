package e2e

import (
	"context"
	"testing"
	"time"
)

func TestRealRuntimeBackendCombosAreResponsive(t *testing.T) {
	options := SuiteOptionsFromEnvironment()
	force := options.ForceReal

	plan, err := PlanRuntimeBackendCombos(options)
	if err != nil {
		t.Fatalf("plan runtime/backend combos: %v", err)
	}
	t.Logf("backend config: %s", plan.ConfigPath)

	if len(plan.Combos) == 0 {
		if force {
			t.Fatalf("no runtime/backend combos to test: %s", plan.SkipReason())
		}
		t.Skip(plan.SkipReason())
	}

	for _, combo := range plan.Combos {
		t.Logf("combo %s: model=%s executable=%s skip=%s", combo.Name(), combo.ModelPath, combo.Executable, combo.SkipReason)
	}

	readyCount := 0
	for _, combo := range plan.Combos {
		combo := combo
		t.Run(combo.Name(), func(t *testing.T) {
			if combo.SkipReason != "" {
				if force {
					t.Fatalf("combo prerequisites missing: %s", combo.SkipReason)
				}
				t.Skip(combo.SkipReason)
			}
			readyCount++

			ctx, cancel := context.WithTimeout(context.Background(), RuntimeBackendE2ETimeout())
			defer cancel()

			result, err := RunRuntimeBackendCombo(ctx, combo)
			if err != nil {
				t.Fatalf("run responsive combo: %v", err)
			}
			t.Logf(
				"combo %s responded through %s with runner %s: %q",
				combo.Name(),
				result.Endpoint,
				result.RunnerID,
				result.ResponseText,
			)
		})
	}

	if readyCount == 0 {
		if force {
			t.Fatalf("no runnable runtime/backend combos: %s", plan.SkipReason())
		}
		t.Skip(plan.SkipReason())
	}
}

func TestRuntimeBackendE2ETimeoutDefaultsToPracticalBound(t *testing.T) {
	t.Setenv(timeoutEnv, "")

	if got := RuntimeBackendE2ETimeout(); got != 4*time.Minute {
		t.Fatalf("default runtime/backend E2E timeout = %v, want 4m", got)
	}
}
