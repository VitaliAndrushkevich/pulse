package notification

import "testing"

func TestValidateHeartbeatTriggers(t *testing.T) {
	tests := []struct {
		name       string
		triggers   []TriggerCondition
		wantErrs   int
		wantFields []string
	}{
		{
			name:       "empty slice requires at least one trigger",
			triggers:   nil,
			wantErrs:   1,
			wantFields: []string{"triggers"},
		},
		{
			name:     "both heartbeat trigger types accepted",
			triggers: []TriggerCondition{{Type: "heartbeat_missed"}, {Type: "heartbeat_recovered"}},
			wantErrs: 0,
		},
		{
			name:       "monitor trigger types rejected",
			triggers:   []TriggerCondition{{Type: "monitor_down"}},
			wantErrs:   1,
			wantFields: []string{"triggers[0].type"},
		},
		{
			name:       "threshold trigger types rejected",
			triggers:   []TriggerCondition{{Type: "degraded"}, {Type: "ssl_expiring"}},
			wantErrs:   2,
			wantFields: []string{"triggers[0].type", "triggers[1].type"},
		},
		{
			name:       "mixed valid and invalid reports only the invalid",
			triggers:   []TriggerCondition{{Type: "heartbeat_missed"}, {Type: "n_failures_in_row"}},
			wantErrs:   1,
			wantFields: []string{"triggers[1].type"},
		},
		{
			name:       "unknown type rejected",
			triggers:   []TriggerCondition{{Type: "bogus"}},
			wantErrs:   1,
			wantFields: []string{"triggers[0].type"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateHeartbeatTriggers(tt.triggers)
			if len(errs) != tt.wantErrs {
				t.Fatalf("expected %d errors, got %d: %+v", tt.wantErrs, len(errs), errs)
			}
			for i, field := range tt.wantFields {
				if errs[i].Field != field {
					t.Errorf("error[%d]: expected field %q, got %q", i, field, errs[i].Field)
				}
				if errs[i].Message == "" {
					t.Errorf("error[%d]: expected a non-empty message", i)
				}
			}
		})
	}
}

func TestTranslateHeartbeatTriggers(t *testing.T) {
	tests := []struct {
		name string
		in   []TriggerCondition
		want []string // expected Type values, in order
	}{
		{
			name: "heartbeat_missed maps to monitor_down",
			in:   []TriggerCondition{{Type: "heartbeat_missed"}},
			want: []string{"monitor_down"},
		},
		{
			name: "heartbeat_recovered maps to monitor_up",
			in:   []TriggerCondition{{Type: "heartbeat_recovered"}},
			want: []string{"monitor_up"},
		},
		{
			name: "whole slice translated in order",
			in:   []TriggerCondition{{Type: "heartbeat_recovered"}, {Type: "heartbeat_missed"}},
			want: []string{"monitor_up", "monitor_down"},
		},
		{
			name: "unknown type passed through unchanged",
			in:   []TriggerCondition{{Type: "monitor_down"}},
			want: []string{"monitor_down"},
		},
		{
			name: "empty slice yields empty slice",
			in:   nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TranslateHeartbeatTriggers(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d conditions, got %d", len(tt.want), len(got))
			}
			for i, want := range tt.want {
				if got[i].Type != want {
					t.Errorf("condition[%d]: expected type %q, got %q", i, want, got[i].Type)
				}
			}
		})
	}
}
