package ds

import (
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxModeNameLength         = 50
	MaxShortDescriptionLength = 600
	MaxSupportPercent         = 1000
	MaxConsumptionWhPerKm     = 1000
)

var allowedTransitions = map[MotorModeStatus][]MotorModeStatus{
	StatusDraft:     {StatusPublished, StatusDeleted},
	StatusPublished: {StatusDeleted},
}

func (s MotorModeStatus) CanChangeTo(next MotorModeStatus) bool {
	return slices.Contains(allowedTransitions[s], next)
}

func ValidText(text string) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}

func ValidateModeName(modeName string) string {
	switch {
	case !ValidText(modeName):
		return "Название режима содержит недопустимые символы."
	case modeName == "":
		return "Укажите название режима работы мотора."
	case utf8.RuneCountInString(modeName) > MaxModeNameLength:
		return "Название режима не длиннее 50 символов."
	}
	return ""
}

func ValidatePublication(shortDescription string, supportPercent *int, consumptionWhPerKm *float64) string {
	switch {
	case !ValidText(shortDescription):
		return "Краткое описание содержит недопустимые символы."
	case shortDescription == "":
		return "Заполните краткое описание режима."
	case utf8.RuneCountInString(shortDescription) > MaxShortDescriptionLength:
		return "Краткое описание не длиннее 600 символов."
	case supportPercent == nil || *supportPercent < 0 || *supportPercent > MaxSupportPercent:
		return "Поддержка мотора задаётся целым числом процентов от 0 до 1000."
	case consumptionWhPerKm == nil || math.IsNaN(*consumptionWhPerKm) || math.IsInf(*consumptionWhPerKm, 0) ||
		*consumptionWhPerKm <= 0 || *consumptionWhPerKm >= MaxConsumptionWhPerKm:
		return "Расход батареи задаётся положительным числом Вт·ч/км."
	}
	return ""
}

func RoundConsumption(consumptionWhPerKm float64) float64 {
	return math.Round(consumptionWhPerKm*10) / 10
}
