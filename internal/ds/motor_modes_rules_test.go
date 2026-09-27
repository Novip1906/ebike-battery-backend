package ds

import "testing"

func TestStatusTransitions(t *testing.T) {
	cases := []struct {
		from, to MotorModeStatus
		allowed  bool
	}{
		{StatusDraft, StatusPublished, true},
		{StatusDraft, StatusDeleted, true},
		{StatusPublished, StatusDeleted, true},
		{StatusPublished, StatusDraft, false},
		{StatusDeleted, StatusDraft, false},
		{StatusDeleted, StatusPublished, false},
		{StatusPublished, StatusPublished, false},
	}
	for _, c := range cases {
		if got := c.from.CanChangeTo(c.to); got != c.allowed {
			t.Errorf("%s → %s: %v, ожидалось %v", c.from, c.to, got, c.allowed)
		}
	}
}

func TestValidatePublication(t *testing.T) {
	support, consumption := 100, 3.4
	if message := ValidatePublication("Помощь при ведении", &support, &consumption); message != "" {
		t.Errorf("корректные поля не прошли: %s", message)
	}
	zero := RoundConsumption(0.04)
	for name, message := range map[string]string{
		"пустое описание":  ValidatePublication("", &support, &consumption),
		"нулевой байт":     ValidatePublication("a\x00b", &support, &consumption),
		"без поддержки":    ValidatePublication("описание", nil, &consumption),
		"расход после 0.1": ValidatePublication("описание", &support, &zero),
	} {
		if message == "" {
			t.Errorf("%s: ошибка не найдена", name)
		}
	}
}

func TestValidateModeName(t *testing.T) {
	for name, modeName := range map[string]string{
		"пустое":       "",
		"длинное":      "12345678901234567890123456789012345678901234567890X",
		"битый UTF-8":  "\xff\xfe",
		"нулевой байт": "Walk\x00",
	} {
		if ValidateModeName(modeName) == "" {
			t.Errorf("%s: название %q прошло проверку", name, modeName)
		}
	}
	if message := ValidateModeName("Walk Assist"); message != "" {
		t.Errorf("корректное название: %s", message)
	}
}
