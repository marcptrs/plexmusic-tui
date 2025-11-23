package util

// SettingsItem represents a configurable setting shown in the settings list.
type SettingsItem struct {
	Group           string
	Name            string
	Key             string
	Kind            string // 'toggle' or 'choice' or 'value'
	Value           interface{}
	DescriptionText string
}

// Implement the list.Item interface used by bubbles/list
func (s SettingsItem) Title() string       { return s.Name }
func (s SettingsItem) Description() string { return s.DescriptionText }
func (s SettingsItem) FilterValue() string { return s.Name }
