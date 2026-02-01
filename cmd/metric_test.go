package cmd

import (
	"testing"
)

func TestMetricCommandStructure(t *testing.T) {
	subcommands := []string{"get", "aggregate"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range metricCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in metric command", name)
		}
	}
}

func TestMetricCommandAliases(t *testing.T) {
	aliases := metricCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "metrics" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'metrics' not found")
	}
}

func TestMetricAggregateCommandAliases(t *testing.T) {
	aliases := metricAggregateCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "agg" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'agg' not found for aggregate command")
	}
}

func TestMetricPersistentFlags(t *testing.T) {
	flags := []string{"format", "output"}

	for _, flag := range flags {
		if metricCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in metric command", flag)
		}
	}
}

func TestMetricGetCommandFlags(t *testing.T) {
	flags := []string{"monitor", "group", "tag", "type", "time", "start", "end", "env", "region", "with-nulls", "field"}

	for _, flag := range flags {
		if metricGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in metric get command", flag)
		}
	}
}

func TestMetricAggregateCommandFlags(t *testing.T) {
	flags := []string{"monitor", "group", "tag", "type", "time", "start", "end", "env", "region", "with-nulls"}

	for _, flag := range flags {
		if metricAggregateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in metric aggregate command", flag)
		}
	}
}

func TestFormatMetricValue(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{float64(100), "100"},
		{float64(99.5), "99.50"},
		{float64(0), "0"},
		{nil, "-"},
		{42, "42"},
		{"test", "test"},
	}

	for _, test := range tests {
		result := formatMetricValue(test.input)
		if result != test.expected {
			t.Errorf("formatMetricValue(%v) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestSplitAndTrimMetric(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"", []string{}},
		{"a,,b", []string{"a", "b"}},
	}

	for _, test := range tests {
		result := splitAndTrimMetric(test.input)
		if len(result) != len(test.expected) {
			t.Errorf("splitAndTrimMetric(%q) returned %d items, expected %d", test.input, len(result), len(test.expected))
			continue
		}
		for i, v := range result {
			if v != test.expected[i] {
				t.Errorf("splitAndTrimMetric(%q)[%d] = %q, expected %q", test.input, i, v, test.expected[i])
			}
		}
	}
}
