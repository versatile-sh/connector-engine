package surface

import "regexp"

// Field is one setting a channel kind takes: its name, English label, placeholder, and whether it is a secret, optional or the one whose last characters hint at the secret.
type Field struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret"`
	Optional    bool   `json:"optional,omitempty"`
	Hint        bool   `json:"-"`
}

// Kind is the single description of a channel kind: how it is named, set up, checked and built, so a new channel is one value and a registration.
type Kind struct {
	Name        string                                   `json:"kind"`
	Title       string                                   `json:"title"`
	Interactive bool                                     `json:"interactive"`
	Fields      []Field                                  `json:"fields"`
	Owner       *regexp.Regexp                           `json:"-"`
	Hosts       []string                                 `json:"-"`
	New         func(map[string]string) (Channel, error) `json:"-"`
}

// Secrets lists the names of the fields that hold a secret.
func (k Kind) Secrets() []string {
	var out []string
	for _, f := range k.Fields {
		if f.Secret {
			out = append(out, f.Name)
		}
	}
	return out
}

// HintField is the name of the secret field whose last characters the settings page shows, or empty when there is none.
func (k Kind) HintField() string {
	for _, f := range k.Fields {
		if f.Hint {
			return f.Name
		}
	}
	return ""
}
