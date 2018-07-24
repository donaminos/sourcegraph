package cxp

import (
	"encoding/json"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/jsonx"
	"github.com/sourcegraph/sourcegraph/xlang/lspext"
)

// ClientProxyInitializeParams contains the parameters for the client's "initialize" request to the
// CXP proxy.
//
// It is lspext.ClientProxyInitializeParams with an added nested initializationOptions.settings
// field.
type ClientProxyInitializeParams struct {
	Root                  lsp.DocumentURI                  `json:"root"`
	InitializationOptions ClientProxyInitializationOptions `json:"initializationOptions"`
	Capabilities          ClientCapabilities               `json:"capabilities"`
	lspext.ClientProxyInitializeParams
}

// RootOrRootURI returns the "root" (CXP), "initializationOptions.rootUri" (LSP backcompat), or
// "rootUri" (LSP backcompat) property value.
func (p ClientProxyInitializeParams) RootOrRootURI() lsp.DocumentURI {
	if p.Root != "" {
		return p.Root
	}
	if p.InitializationOptions.ClientProxyInitializationOptions.RootURI != nil {
		return *p.InitializationOptions.ClientProxyInitializationOptions.RootURI
	}
	return p.RootURI
}

// ClientProxyInitializeParams contains the initialization options for the client's "initialize"
// request to the CXP proxy.
type ClientProxyInitializationOptions struct {
	lspext.ClientProxyInitializationOptions
	Settings ExtensionSettings `json:"settings"`
}

// InitializeParams contains the parameters for the client's (or proxy's) "initialize" request to
// the extension.
//
// It is lspext.InitializeParams with an added nested initializationOptions.settings field.
type InitializeParams struct {
	Root                  lsp.DocumentURI        `json:"root"`
	InitializationOptions *InitializationOptions `json:"initializationOptions,omitempty"`
	Capabilities          ClientCapabilities     `json:"capabilities"`
	lspext.InitializeParams
}

// RootOrRootURI returns the "root" (CXP) or "rootUri" (LSP backcompat) property value.
func (p InitializeParams) RootOrRootURI() lsp.DocumentURI {
	if p.Root != "" {
		return p.Root
	}
	return p.RootURI
}

// InitializationOptions contains arbitrary initialization options at the top level, plus extension
// settings.
type InitializationOptions struct {
	Other    map[string]interface{} `json:"-"`
	Settings ExtensionSettings      `json:"settings"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *InitializationOptions) UnmarshalJSON(data []byte) error {
	var s struct {
		Settings ExtensionSettings `json:"settings"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*o = InitializationOptions{Settings: s.Settings}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	delete(m, "settings")

	if len(m) > 0 {
		(*o).Other = make(map[string]interface{}, len(m))
	}
	for k, v := range m {
		(*o).Other[k] = v
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (o InitializationOptions) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{}, len(o.Other)+1)
	for k, v := range o.Other {
		m[k] = v
	}
	m["settings"] = o.Settings
	return json.Marshal(m)
}

// ExtensionSettings contains the global/organization/user settings for an extension.
type ExtensionSettings struct {
	Merged *json.RawMessage `json:"merged,omitempty"`
}

type ClientCapabilities struct {
	lsp.ClientCapabilities

	Decoration *DecorationCapabilityOptions `json:"decoration,omitempty"`

	// TODO(sqs): add this to cxp-js
	Exec bool `json:"exec"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	lsp.InitializeResult
}

type ServerCapabilities struct {
	lsp.ServerCapabilities

	DecorationProvider *DecorationProviderServerCapabilities `json:"decorationProvider,omitempty"`

	Contributions *Contributions `json:"contributions,omitempty"`
}

type DecorationCapabilityOptions struct {
	Static  bool `json:"static,omitempty"`
	Dynamic bool `json:"dynamic,omitempty"`
}

type DecorationProviderServerCapabilities struct {
	DecorationCapabilityOptions
}

type TextDocumentPublishDecorationsParams struct {
	TextDocument lsp.TextDocumentIdentifier      `json:"textDocument"`
	Decorations  []lspext.TextDocumentDecoration `json:"decorations"`
}

// ParseClientCapabilities parses the client capabilities from the client's initialize message.
func ParseClientCapabilities(initializeParams []byte) (*ClientCapabilities, error) {
	var params struct {
		Capabilities ClientCapabilities `json:"capabilities"`
	}
	if err := json.Unmarshal(initializeParams, &params); err != nil {
		return nil, err
	}
	return &params.Capabilities, nil
}

type ConfigurationUpdateParams struct {
	Path  jsonx.Path  `json:"path"`
	Value interface{} `json:"value"`
}

type Registration struct {
	ID                string      `json:"id"`
	Method            string      `json:"method"`
	RegisterOptions   interface{} `json:"registerOptions,omitempty"`
	OverwriteExisting bool        `json:"overwriteExisting,omitempty"`
}

type RegistrationParams struct {
	Registrations []Registration `json:"registrations"`
}

type Unregistration struct {
	ID     string `json:"id"`
	Method string `json:"method"`
}

type UnregistrationParams struct {
	// NOTE: The JSON field name typo exists in LSP ("unregisterations", the commonly used English
	// spelling is "unregistrations").
	Unregistrations []Unregistration `json:"unregisterations,omitempty"`
}

type ShowInputParams struct {
	Message      string `json:"message"`
	DefaultValue string `json:"defaultValue,omitempty"`
}
