package transform

import "encoding/json"

const (
	PluginInvalidInputError = "PluginInvalidInputError"
	PluginRunError          = "PluginRunError"
	PluginInvalidIOError    = "PluginInvalidIOError"
)

type PluginError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Err     error  `json:"error"`
}

func (p *PluginError) Error() string {
	b, _ := json.Marshal(p)
	return string(b)
}

func IsInvalidInputError(err error) bool {
	perr, ok := err.(*PluginError)
	if !ok {
		return false
	}
	return perr.Type == PluginInvalidInputError
}

func IsPluginRunError(err error) bool {
	perr, ok := err.(*PluginError)
	if !ok {
		return false
	}
	return perr.Type == PluginRunError
}

func IsInvalidIOError(err error) bool {
	perr, ok := err.(*PluginError)
	if !ok {
		return false
	}
	return perr.Type == PluginInvalidIOError
}
