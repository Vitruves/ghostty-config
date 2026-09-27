package tui

import "github.com/charmbracelet/bubbles/textinput"

// The dialogs share the bubbles text input under a short alias.
type textinputModel = textinput.Model

func textinputNew() textinput.Model { return textinput.New() }
