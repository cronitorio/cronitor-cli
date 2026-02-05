//go:build windows
// +build windows

package cmd

import (
	"testing"
	"time"

	"github.com/capnspacehook/taskmaster"
	"github.com/rickb777/date/period"
)

// Test helper functions

func TestConvertDaysOfWeek(t *testing.T) {
	tests := []struct {
		name     string
		days     taskmaster.DayOfWeek
		expected string
	}{
		{
			name:     "Monday only",
			days:     taskmaster.Monday,
			expected: "MO",
		},
		{
			name:     "Monday, Wednesday, Friday",
			days:     taskmaster.Monday | taskmaster.Wednesday | taskmaster.Friday,
			expected: "MO,WE,FR",
		},
		{
			name:     "All weekdays",
			days:     taskmaster.Monday | taskmaster.Tuesday | taskmaster.Wednesday | taskmaster.Thursday | taskmaster.Friday,
			expected: "MO,TU,WE,TH,FR",
		},
		{
			name:     "Weekend only",
			days:     taskmaster.Saturday | taskmaster.Sunday,
			expected: "SA,SU",
		},
		{
			name:     "All days",
			days:     taskmaster.Monday | taskmaster.Tuesday | taskmaster.Wednesday | taskmaster.Thursday | taskmaster.Friday | taskmaster.Saturday | taskmaster.Sunday,
			expected: "MO,TU,WE,TH,FR,SA,SU",
		},
		{
			name:     "No days",
			days:     0,
			expected: "",
		},
		{
			name:     "Tuesday and Thursday",
			days:     taskmaster.Tuesday | taskmaster.Thursday,
			expected: "TU,TH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertDaysOfWeek(tt.days)
			if result != tt.expected {
				t.Errorf("convertDaysOfWeek() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertMonthDays(t *testing.T) {
	tests := []struct {
		name     string
		days     taskmaster.DayOfMonth
		expected string
	}{
		{
			name:     "First day of month",
			days:     1 << 0, // Day 1
			expected: "1",
		},
		{
			name:     "1st and 15th",
			days:     (1 << 0) | (1 << 14), // Days 1 and 15
			expected: "1,15",
		},
		{
			name:     "Last day of month (31st)",
			days:     1 << 30, // Day 31
			expected: "31",
		},
		{
			name:     "Multiple days",
			days:     (1 << 0) | (1 << 9) | (1 << 19) | (1 << 29), // 1st, 10th, 20th, 30th
			expected: "1,10,20,30",
		},
		{
			name:     "No days",
			days:     0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMonthDays(tt.days)
			if result != tt.expected {
				t.Errorf("convertMonthDays() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertWeekOfMonth(t *testing.T) {
	tests := []struct {
		name     string
		weeks    taskmaster.Week
		days     taskmaster.DayOfWeek
		expected string
	}{
		{
			name:     "First Monday",
			weeks:    taskmaster.First,
			days:     taskmaster.Monday,
			expected: "1MO",
		},
		{
			name:     "Second Tuesday",
			weeks:    taskmaster.Second,
			days:     taskmaster.Tuesday,
			expected: "2TU",
		},
		{
			name:     "Last Friday",
			weeks:    taskmaster.LastWeek,
			days:     taskmaster.Friday,
			expected: "-1FR",
		},
		{
			name:     "Third Wednesday",
			weeks:    taskmaster.Third,
			days:     taskmaster.Wednesday,
			expected: "3WE",
		},
		{
			name:     "Fourth Thursday",
			weeks:    taskmaster.Fourth,
			days:     taskmaster.Thursday,
			expected: "4TH",
		},
		{
			name:     "First Monday and Wednesday",
			weeks:    taskmaster.First,
			days:     taskmaster.Monday | taskmaster.Wednesday,
			expected: "1MO,1WE",
		},
		{
			name:     "Last weekday (Mon-Fri)",
			weeks:    taskmaster.LastWeek,
			days:     taskmaster.Monday | taskmaster.Tuesday | taskmaster.Wednesday | taskmaster.Thursday | taskmaster.Friday,
			expected: "-1MO,-1TU,-1WE,-1TH,-1FR",
		},
		{
			name:     "No weeks or days",
			weeks:    0,
			days:     0,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertWeekOfMonth(tt.weeks, tt.days)
			if result != tt.expected {
				t.Errorf("convertWeekOfMonth() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractTimeComponents(t *testing.T) {
	tests := []struct {
		name         string
		time         time.Time
		expectedHour int
		expectedMin  int
	}{
		{
			name:         "9:30 AM",
			time:         time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC),
			expectedHour: 9,
			expectedMin:  30,
		},
		{
			name:         "Midnight",
			time:         time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			expectedHour: 0,
			expectedMin:  0,
		},
		{
			name:         "11:59 PM",
			time:         time.Date(2025, 1, 1, 23, 59, 0, 0, time.UTC),
			expectedHour: 23,
			expectedMin:  59,
		},
		{
			name:         "2:15 PM",
			time:         time.Date(2025, 1, 1, 14, 15, 0, 0, time.UTC),
			expectedHour: 14,
			expectedMin:  15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour, min := extractTimeComponents(tt.time)
			if hour != tt.expectedHour || min != tt.expectedMin {
				t.Errorf("extractTimeComponents() = (%v, %v), want (%v, %v)",
					hour, min, tt.expectedHour, tt.expectedMin)
			}
		})
	}
}

// Test trigger conversions

func TestConvertTriggerToRRULE_DailyTrigger(t *testing.T) {
	tests := []struct {
		name         string
		trigger      taskmaster.DailyTrigger
		expectedRule string
	}{
		{
			name: "Daily at 9:30 AM",
			trigger: taskmaster.DailyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC),
				},
				DayInterval: 1,
				RandomDelay: period.Period{},
			},
			expectedRule: "FREQ=DAILY;INTERVAL=1;BYHOUR=9;BYMINUTE=30",
		},
		{
			name: "Every 2 days at 14:00",
			trigger: taskmaster.DailyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
				},
				DayInterval: 2,
				RandomDelay: period.Period{},
			},
			expectedRule: "FREQ=DAILY;INTERVAL=2;BYHOUR=14",
		},
		{
			name: "Daily with disabled trigger",
			trigger: taskmaster.DailyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       false,
					StartBoundary: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC),
				},
				DayInterval: 1,
				RandomDelay: period.Period{},
			},
			expectedRule: "FREQ=DAILY;INTERVAL=1;BYHOUR=9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := convertTriggerToRRULE(tt.trigger)
			if info.RRULE != tt.expectedRule {
				t.Errorf("convertTriggerToRRULE() rrule = %v, want %v", info.RRULE, tt.expectedRule)
			}
		})
	}
}

func TestConvertTriggerToRRULE_WeeklyTrigger(t *testing.T) {
	tests := []struct {
		name         string
		trigger      taskmaster.WeeklyTrigger
		expectedRule string
	}{
		{
			name: "Weekly Monday, Wednesday, Friday at 14:00",
			trigger: taskmaster.WeeklyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Monday | taskmaster.Wednesday | taskmaster.Friday,
				WeekInterval: 1,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE,FR;BYHOUR=14",
		},
		{
			name: "Every 2 weeks on Tuesday at 10:00",
			trigger: taskmaster.WeeklyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Tuesday,
				WeekInterval: 2,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU;BYHOUR=10",
		},
		{
			name: "Weekends only",
			trigger: taskmaster.WeeklyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 8, 30, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Saturday | taskmaster.Sunday,
				WeekInterval: 1,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=SA,SU;BYHOUR=8;BYMINUTE=30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := convertTriggerToRRULE(tt.trigger)
			if info.RRULE != tt.expectedRule {
				t.Errorf("convertTriggerToRRULE() rrule = %v, want %v", info.RRULE, tt.expectedRule)
			}
		})
	}
}

func TestConvertTriggerToRRULE_MonthlyTrigger(t *testing.T) {
	tests := []struct {
		name         string
		trigger      taskmaster.MonthlyTrigger
		expectedRule string
	}{
		{
			name: "1st and 15th of month at midnight",
			trigger: taskmaster.MonthlyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				DaysOfMonth: (1 << 0) | (1 << 14), // 1st and 15th
				RandomDelay: period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYMONTHDAY=1,15;BYHOUR=0",
		},
		{
			name: "Last day of month at 23:59",
			trigger: taskmaster.MonthlyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 23, 59, 0, 0, time.UTC),
				},
				DaysOfMonth:          1 << 30, // 31st
				RunOnLastWeekOfMonth: true,
				RandomDelay:          period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYMONTHDAY=31,-1;BYHOUR=23;BYMINUTE=59",
		},
		{
			name: "First day of month",
			trigger: taskmaster.MonthlyTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				DaysOfMonth: 1 << 0, // 1st
				RandomDelay: period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYMONTHDAY=1;BYHOUR=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := convertTriggerToRRULE(tt.trigger)
			if info.RRULE != tt.expectedRule {
				t.Errorf("convertTriggerToRRULE() rrule = %v, want %v", info.RRULE, tt.expectedRule)
			}
		})
	}
}

func TestConvertTriggerToRRULE_MonthlyDOWTrigger(t *testing.T) {
	tests := []struct {
		name         string
		trigger      taskmaster.MonthlyDOWTrigger
		expectedRule string
	}{
		{
			name: "Second Tuesday at 9:00 AM (Patch Tuesday)",
			trigger: taskmaster.MonthlyDOWTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Tuesday,
				WeeksOfMonth: taskmaster.Second,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYDAY=2TU;BYHOUR=9",
		},
		{
			name: "Last Friday at 17:00",
			trigger: taskmaster.MonthlyDOWTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 17, 0, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Friday,
				WeeksOfMonth: taskmaster.LastWeek,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYDAY=-1FR;BYHOUR=17",
		},
		{
			name: "First Monday at 8:00 AM",
			trigger: taskmaster.MonthlyDOWTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Monday,
				WeeksOfMonth: taskmaster.First,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYDAY=1MO;BYHOUR=8",
		},
		{
			name: "Third Wednesday at 14:30",
			trigger: taskmaster.MonthlyDOWTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 1, 1, 14, 30, 0, 0, time.UTC),
				},
				DaysOfWeek:   taskmaster.Wednesday,
				WeeksOfMonth: taskmaster.Third,
				RandomDelay:  period.Period{},
			},
			expectedRule: "FREQ=MONTHLY;BYDAY=3WE;BYHOUR=14;BYMINUTE=30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := convertTriggerToRRULE(tt.trigger)
			if info.RRULE != tt.expectedRule {
				t.Errorf("convertTriggerToRRULE() rrule = %v, want %v", info.RRULE, tt.expectedRule)
			}
		})
	}
}

func TestConvertTriggerToRRULE_EventDrivenTriggers(t *testing.T) {
	tests := []struct {
		name               string
		trigger            taskmaster.Trigger
		expectedRule       string
		expectedDescPrefix string // Check that description starts with this
	}{
		{
			name: "Boot trigger with delay",
			trigger: taskmaster.BootTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled: true,
				},
				Delay: period.MustParse("PT5M"),
			},
			expectedRule:       "",
			expectedDescPrefix: "Runs on system boot",
		},
		{
			name: "Logon trigger with user",
			trigger: taskmaster.LogonTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled: true,
				},
				UserID: "DOMAIN\\username",
				Delay:  period.Period{},
			},
			expectedRule:       "",
			expectedDescPrefix: "Runs on user logon",
		},
		{
			name: "Idle trigger",
			trigger: taskmaster.IdleTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled: true,
				},
			},
			expectedRule:       "",
			expectedDescPrefix: "Runs when system is idle",
		},
		{
			name: "Registration trigger",
			trigger: taskmaster.RegistrationTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled: true,
				},
				Delay: period.Period{},
			},
			expectedRule:       "",
			expectedDescPrefix: "Runs when task is registered",
		},
		{
			name: "Time trigger (one-time)",
			trigger: taskmaster.TimeTrigger{
				TaskTrigger: taskmaster.TaskTrigger{
					Enabled:       true,
					StartBoundary: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
				},
				RandomDelay: period.Period{},
			},
			expectedRule:       "",
			expectedDescPrefix: "Runs once at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := convertTriggerToRRULE(tt.trigger)

			if info.RRULE != tt.expectedRule {
				t.Errorf("convertTriggerToRRULE() rrule = %v, want %v", info.RRULE, tt.expectedRule)
			}

			if info.Description == "" {
				t.Error("Expected description for event-driven trigger")
			}

			if len(info.Description) < len(tt.expectedDescPrefix) ||
			   info.Description[:len(tt.expectedDescPrefix)] != tt.expectedDescPrefix {
				t.Errorf("Description = %v, expected to start with %v", info.Description, tt.expectedDescPrefix)
			}
		})
	}
}

func TestConvertTriggerToRRULE_WithBoundaries(t *testing.T) {
	expectedStart := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: expectedStart,
			EndBoundary:   expectedEnd,
		},
		DayInterval: 1,
		RandomDelay: period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=DAILY;INTERVAL=1;BYHOUR=9"
	if info.RRULE != expectedRule {
		t.Errorf("rrule = %v, want %v", info.RRULE, expectedRule)
	}

	if info.StartBoundary.IsZero() {
		t.Error("Expected StartBoundary to be set")
	}

	if !info.StartBoundary.Equal(expectedStart) {
		t.Errorf("StartBoundary = %v, want %v", info.StartBoundary, expectedStart)
	}

	if info.EndBoundary.IsZero() {
		t.Error("Expected EndBoundary to be set")
	}

	if !info.EndBoundary.Equal(expectedEnd) {
		t.Errorf("EndBoundary = %v, want %v", info.EndBoundary, expectedEnd)
	}
}

func TestConvertTriggerToRRULE_WithRandomDelay(t *testing.T) {
	// Test that trigger with random delay still generates valid RRULE
	// Random delay is preserved in the trigger but doesn't affect RRULE generation
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC),
		},
		DayInterval: 1,
		RandomDelay: period.MustParse("PT30M"),
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=DAILY;INTERVAL=1;BYHOUR=9"
	if info.RRULE != expectedRule {
		t.Errorf("rrule = %v, want %v", info.RRULE, expectedRule)
	}
}

// Edge case tests

func TestConvertTriggerToRRULE_MidnightTime(t *testing.T) {
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		DayInterval: 1,
		RandomDelay: period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	expected := "FREQ=DAILY;INTERVAL=1;BYHOUR=0"
	if info.RRULE != expected {
		t.Errorf("Midnight time: rrule = %v, want %v", info.RRULE, expected)
	}
}

func TestConvertTriggerToRRULE_ZeroStartBoundary(t *testing.T) {
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Time{}, // Zero value
		},
		DayInterval: 1,
		RandomDelay: period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	// Should still generate RRULE but without time components
	expected := "FREQ=DAILY;INTERVAL=1"
	if info.RRULE != expected {
		t.Errorf("Zero start boundary: rrule = %v, want %v", info.RRULE, expected)
	}

	// Should have zero StartBoundary
	if !info.StartBoundary.IsZero() {
		t.Errorf("Expected zero StartBoundary, got %v", info.StartBoundary)
	}
}

func TestConvertTriggerToRRULE_LargeInterval(t *testing.T) {
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC),
		},
		DayInterval: 30,
		RandomDelay: period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	expected := "FREQ=DAILY;INTERVAL=30;BYHOUR=9"
	if info.RRULE != expected {
		t.Errorf("Large interval: rrule = %v, want %v", info.RRULE, expected)
	}
}

func TestConvertTriggerToRRULE_AllDaysOfWeek(t *testing.T) {
	trigger := taskmaster.WeeklyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		DaysOfWeek: taskmaster.Monday | taskmaster.Tuesday | taskmaster.Wednesday |
			taskmaster.Thursday | taskmaster.Friday | taskmaster.Saturday | taskmaster.Sunday,
		WeekInterval: 1,
		RandomDelay:  period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	// Should include all 7 days
	expected := "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR,SA,SU;BYHOUR=10"
	if info.RRULE != expected {
		t.Errorf("All days: rrule = %v, want %v", info.RRULE, expected)
	}
}

func TestConvertTriggerToRRULE_NoDaysOfWeek(t *testing.T) {
	trigger := taskmaster.WeeklyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		DaysOfWeek:   0, // No days specified
		WeekInterval: 1,
		RandomDelay:  period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	// Should generate RRULE without BYDAY
	expected := "FREQ=WEEKLY;INTERVAL=1;BYHOUR=10"
	if info.RRULE != expected {
		t.Errorf("No days: rrule = %v, want %v", info.RRULE, expected)
	}
}

func TestConvertTriggerToRRULE_SessionStateChange(t *testing.T) {
	trigger := taskmaster.SessionStateChangeTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled: true,
		},
		StateChange: taskmaster.TASK_SESSION_LOCK,
		UserId:      "DOMAIN\\user",
		Delay:       period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	if info.RRULE != "" {
		t.Errorf("Expected empty rrule for session state change, got %v", info.RRULE)
	}

	if info.Description == "" {
		t.Error("Expected description for session state change trigger")
	}

	// Should mention session state change in description
	if len(info.Description) < 7 || info.Description[:7] != "Runs on" {
		t.Errorf("Description = %v, expected to describe event-driven trigger", info.Description)
	}
}

// RRULE format validation tests

func TestRRULEFormat_ValidComponents(t *testing.T) {
	// Test that generated RRULEs only contain valid components
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC),
		},
		DayInterval: 1,
		RandomDelay: period.Period{},
	}

	info := convertTriggerToRRULE(trigger)
	rrule := info.RRULE

	// RRULE should not have trailing semicolons
	if rrule[len(rrule)-1] == ';' {
		t.Error("RRULE should not end with semicolon")
	}

	// RRULE should start with FREQ=
	if rrule[:5] != "FREQ=" {
		t.Error("RRULE should start with FREQ=")
	}

	// RRULE components should be separated by semicolons
	components := map[string]bool{
		"FREQ":     false,
		"INTERVAL": false,
		"BYHOUR":   false,
		"BYMINUTE": false,
	}

	for component := range components {
		if len(rrule) > len(component) {
			// Simple check if component exists
			for i := 0; i < len(rrule)-len(component); i++ {
				if rrule[i:i+len(component)] == component {
					components[component] = true
					break
				}
			}
		}
	}

	if !components["FREQ"] {
		t.Error("RRULE missing required FREQ component")
	}

	if !components["INTERVAL"] {
		t.Error("RRULE missing INTERVAL component")
	}
}

func TestRRULEFormat_NoSpaces(t *testing.T) {
	trigger := taskmaster.WeeklyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 14, 0, 0, 0, time.UTC),
		},
		DaysOfWeek:   taskmaster.Monday | taskmaster.Wednesday | taskmaster.Friday,
		WeekInterval: 1,
		RandomDelay:  period.Period{},
	}

	info := convertTriggerToRRULE(trigger)
	rrule := info.RRULE

	// RRULE should not contain spaces
	for _, char := range rrule {
		if char == ' ' {
			t.Errorf("RRULE contains spaces: %v", rrule)
			break
		}
	}
}

func TestRRULEFormat_DayAbbreviations(t *testing.T) {
	trigger := taskmaster.WeeklyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		DaysOfWeek:   taskmaster.Monday | taskmaster.Tuesday,
		WeekInterval: 1,
		RandomDelay:  period.Period{},
	}

	info := convertTriggerToRRULE(trigger)
	rrule := info.RRULE

	// Check for correct 2-letter day abbreviations
	validDays := []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}
	hasDays := false

	for _, day := range validDays {
		for i := 0; i < len(rrule)-1; i++ {
			if rrule[i:i+2] == day {
				hasDays = true
				break
			}
		}
	}

	if !hasDays {
		t.Errorf("RRULE should contain day abbreviations: %v", rrule)
	}
}

// Comprehensive scenario tests

func TestCompleteScenario_PatchTuesday(t *testing.T) {
	// Real-world scenario: Patch Tuesday (2nd Tuesday of month at 10 PM)
	trigger := taskmaster.MonthlyDOWTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 14, 22, 0, 0, 0, time.UTC),
		},
		DaysOfWeek:   taskmaster.Tuesday,
		WeeksOfMonth: taskmaster.Second,
		RandomDelay:  period.MustParse("PT1H"), // 1 hour random delay
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=MONTHLY;BYDAY=2TU;BYHOUR=22"
	if info.RRULE != expectedRule {
		t.Errorf("Patch Tuesday rrule = %v, want %v", info.RRULE, expectedRule)
	}

	if info.StartBoundary.IsZero() {
		t.Error("Expected StartBoundary for Patch Tuesday")
	}
}

func TestCompleteScenario_DailyBackup(t *testing.T) {
	// Real-world scenario: Daily backup at 2 AM with 30 min random delay
	trigger := taskmaster.DailyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC),
			EndBoundary:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		DayInterval: 1,
		RandomDelay: period.MustParse("PT30M"),
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=DAILY;INTERVAL=1;BYHOUR=2"
	if info.RRULE != expectedRule {
		t.Errorf("Daily backup rrule = %v, want %v", info.RRULE, expectedRule)
	}

	if info.StartBoundary.IsZero() || info.EndBoundary.IsZero() {
		t.Error("Expected both start and end boundaries")
	}
}

func TestCompleteScenario_WeekdayMaintenance(t *testing.T) {
	// Real-world scenario: Maintenance window Mon-Fri at 6 AM
	trigger := taskmaster.WeeklyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC),
		},
		DaysOfWeek: taskmaster.Monday | taskmaster.Tuesday | taskmaster.Wednesday |
			taskmaster.Thursday | taskmaster.Friday,
		WeekInterval: 1,
		RandomDelay:  period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,TU,WE,TH,FR;BYHOUR=6"
	if info.RRULE != expectedRule {
		t.Errorf("Weekday maintenance rrule = %v, want %v", info.RRULE, expectedRule)
	}
}

func TestCompleteScenario_MonthEndReport(t *testing.T) {
	// Real-world scenario: Month-end report on last day at 11:59 PM
	trigger := taskmaster.MonthlyTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled:       true,
			StartBoundary: time.Date(2025, 1, 31, 23, 59, 0, 0, time.UTC),
		},
		DaysOfMonth:          1 << 30, // 31st
		RunOnLastWeekOfMonth: true,
		RandomDelay:          period.Period{},
	}

	info := convertTriggerToRRULE(trigger)

	expectedRule := "FREQ=MONTHLY;BYMONTHDAY=31,-1;BYHOUR=23;BYMINUTE=59"
	if info.RRULE != expectedRule {
		t.Errorf("Month-end report rrule = %v, want %v", info.RRULE, expectedRule)
	}
}

func TestCompleteScenario_StartupTask(t *testing.T) {
	// Real-world scenario: Run on boot with 5 minute delay
	trigger := taskmaster.BootTrigger{
		TaskTrigger: taskmaster.TaskTrigger{
			Enabled: true,
		},
		Delay: period.MustParse("PT5M"),
	}

	info := convertTriggerToRRULE(trigger)

	if info.RRULE != "" {
		t.Error("Boot trigger should not have RRULE")
	}

	if info.Description == "" {
		t.Error("Boot trigger should have description")
	}

	// Should mention boot and delay in description
	if len(info.Description) < 13 || info.Description[:13] != "Runs on system boot" {
		t.Errorf("Description = %v, expected to mention boot", info.Description)
	}
}

// Task filtering tests

func TestShouldIncludeTask_ValidDailyTrigger(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.DailyTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					DayInterval: 1,
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with enabled daily trigger")
	}
}

func TestShouldIncludeTask_DisabledTrigger(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.DailyTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: false, // Disabled
					},
					DayInterval: 1,
				},
			},
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with only disabled triggers")
	}
}

func TestShouldIncludeTask_OneTimeTrigger(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.TimeTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled:       true,
						StartBoundary: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with only one-time triggers")
	}
}

func TestShouldIncludeTask_SessionStateChangeTrigger(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.SessionStateChangeTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					StateChange: taskmaster.TASK_SESSION_LOCK,
				},
			},
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with only session state change triggers")
	}
}

func TestShouldIncludeTask_NoTriggers(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{},
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with no triggers")
	}
}

func TestShouldIncludeTask_NilTriggers(t *testing.T) {
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: nil,
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with nil triggers")
	}
}

func TestShouldIncludeTask_MixedTriggers_WithValid(t *testing.T) {
	// Task has both disabled and enabled triggers - should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.DailyTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: false, // Disabled
					},
					DayInterval: 1,
				},
				taskmaster.WeeklyTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true, // Enabled
					},
					DaysOfWeek:   taskmaster.Monday,
					WeekInterval: 1,
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with at least one valid trigger")
	}
}

func TestShouldIncludeTask_MixedTriggers_OneTimeAndValid(t *testing.T) {
	// Task has both one-time and recurring triggers - should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.TimeTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled:       true,
						StartBoundary: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					},
				},
				taskmaster.DailyTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					DayInterval: 1,
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with at least one recurring trigger")
	}
}

func TestShouldIncludeTask_AllInvalidTypes(t *testing.T) {
	// Task has multiple triggers but all are invalid types
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.TimeTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled:       true,
						StartBoundary: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
					},
				},
				taskmaster.SessionStateChangeTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					StateChange: taskmaster.TASK_SESSION_LOCK,
				},
			},
		},
	}

	if shouldIncludeTask(task) {
		t.Error("Should skip task with only invalid trigger types")
	}
}

func TestShouldIncludeTask_BootTrigger(t *testing.T) {
	// Boot triggers should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.BootTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					Delay: period.MustParse("PT5M"),
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with boot trigger")
	}
}

func TestShouldIncludeTask_LogonTrigger(t *testing.T) {
	// Logon triggers should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.LogonTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
					UserID: "DOMAIN\\user",
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with logon trigger")
	}
}

func TestShouldIncludeTask_IdleTrigger(t *testing.T) {
	// Idle triggers should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.IdleTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with idle trigger")
	}
}

func TestShouldIncludeTask_RegistrationTrigger(t *testing.T) {
	// Registration triggers should be included
	task := taskmaster.RegisteredTask{
		Definition: taskmaster.Definition{
			Triggers: []taskmaster.Trigger{
				taskmaster.RegistrationTrigger{
					TaskTrigger: taskmaster.TaskTrigger{
						Enabled: true,
					},
				},
			},
		},
	}

	if !shouldIncludeTask(task) {
		t.Error("Should include task with registration trigger")
	}
}

func TestShouldIncludeTask_AllRecurringTypes(t *testing.T) {
	// Test all valid recurring trigger types
	triggerTypes := []taskmaster.Trigger{
		taskmaster.DailyTrigger{
			TaskTrigger: taskmaster.TaskTrigger{Enabled: true},
			DayInterval: 1,
		},
		taskmaster.WeeklyTrigger{
			TaskTrigger:  taskmaster.TaskTrigger{Enabled: true},
			DaysOfWeek:   taskmaster.Monday,
			WeekInterval: 1,
		},
		taskmaster.MonthlyTrigger{
			TaskTrigger: taskmaster.TaskTrigger{Enabled: true},
			DaysOfMonth: 1 << 0,
		},
		taskmaster.MonthlyDOWTrigger{
			TaskTrigger:  taskmaster.TaskTrigger{Enabled: true},
			DaysOfWeek:   taskmaster.Monday,
			WeeksOfMonth: taskmaster.First,
		},
	}

	for i, trigger := range triggerTypes {
		task := taskmaster.RegisteredTask{
			Definition: taskmaster.Definition{
				Triggers: []taskmaster.Trigger{trigger},
			},
		}

		if !shouldIncludeTask(task) {
			t.Errorf("Test case %d: Should include recurring trigger type", i)
		}
	}
}
