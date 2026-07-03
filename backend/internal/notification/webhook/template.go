// Package webhook implements webhook template rendering and validation
// for the Pulse notification subsystem.
package webhook

import (
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
)

// knownTemplateVars is the complete set of valid template variable paths
// available in webhook body templates. These correspond to the fields
// of notification.TemplateData and its nested structs.
var knownTemplateVars = map[string]bool{
	"Monitor.ID":        true,
	"Monitor.Name":      true,
	"Monitor.URL":       true,
	"Monitor.Target":    true,
	"Status":            true,
	"PreviousStatus":    true,
	"ResponseTime":      true,
	"Incident.StartedAt": true,
	"Incident.Duration": true,
	"Incident.ID":       true,
	"Timestamp":         true,
	"BaseURL":           true,
}

// ValidateWebhookTemplate parses tmplStr as a Go text/template and verifies
// that all referenced variables belong to the known Template_Variable set.
// Returns nil if the template is valid, or a descriptive error for parse
// failures or unknown variable references.
func ValidateWebhookTemplate(tmplStr string) error {
	tmpl, err := template.New("webhook").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("template parse error: %w", err)
	}

	// Walk the template tree to extract referenced variables
	// and validate against the known TemplateVariable set.
	vars := extractTemplateVars(tmpl)
	for _, v := range vars {
		if !isKnownTemplateVar(v) {
			return fmt.Errorf("unknown template variable: %s", v)
		}
	}
	return nil
}

// extractTemplateVars walks the parsed template AST and collects all
// dot-prefixed field references (e.g., .Monitor.Name becomes "Monitor.Name").
func extractTemplateVars(tmpl *template.Template) []string {
	var vars []string
	if tmpl.Tree == nil {
		return vars
	}
	walkNode(tmpl.Tree.Root, &vars)
	return vars
}

// walkNode recursively traverses template parse nodes to find field references.
func walkNode(node parse.Node, vars *[]string) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			walkNode(child, vars)
		}

	case *parse.ActionNode:
		if n.Pipe != nil {
			walkPipe(n.Pipe, vars)
		}

	case *parse.IfNode:
		walkBranch(&n.BranchNode, vars)

	case *parse.RangeNode:
		walkBranch(&n.BranchNode, vars)

	case *parse.WithNode:
		walkBranch(&n.BranchNode, vars)

	case *parse.TemplateNode:
		if n.Pipe != nil {
			walkPipe(n.Pipe, vars)
		}
	}
}

// walkBranch handles if/range/with branch nodes which contain a pipe and
// list nodes for the body and else clause.
func walkBranch(branch *parse.BranchNode, vars *[]string) {
	if branch.Pipe != nil {
		walkPipe(branch.Pipe, vars)
	}
	if branch.List != nil {
		walkNode(branch.List, vars)
	}
	if branch.ElseList != nil {
		walkNode(branch.ElseList, vars)
	}
}

// walkPipe extracts field references from pipe commands.
func walkPipe(pipe *parse.PipeNode, vars *[]string) {
	if pipe == nil {
		return
	}
	for _, cmd := range pipe.Cmds {
		for _, arg := range cmd.Args {
			extractFieldChain(arg, vars)
		}
	}
}

// extractFieldChain resolves a chain of field accesses starting from dot (.)
// into a dot-separated variable path. For example, {{.Monitor.Name}} produces
// "Monitor.Name".
func extractFieldChain(node parse.Node, vars *[]string) {
	switch n := node.(type) {
	case *parse.FieldNode:
		// FieldNode.Ident contains the chain of identifiers after the dot.
		// e.g., {{.Monitor.Name}} → Ident = ["Monitor", "Name"]
		if len(n.Ident) > 0 {
			*vars = append(*vars, strings.Join(n.Ident, "."))
		}

	case *parse.ChainNode:
		// ChainNode represents a sequence of field accesses on an expression.
		// Walk the node it chains from as well.
		extractFieldChain(n.Node, vars)
		if len(n.Field) > 0 {
			*vars = append(*vars, strings.Join(n.Field, "."))
		}

	case *parse.PipeNode:
		walkPipe(n, vars)
	}
}

// isKnownTemplateVar checks whether a variable path is in the known set.
// It accepts both the full path (e.g., "Monitor.Name") and any prefix
// that corresponds to a struct field (e.g., "Monitor" is valid because
// MonitorData is a valid struct access).
func isKnownTemplateVar(v string) bool {
	// Exact match against the full known variable set.
	if knownTemplateVars[v] {
		return true
	}

	// Check if v is a valid prefix (struct-level access like .Monitor or .Incident).
	// This handles cases like {{.Monitor}} or {{.Incident}} which are valid
	// Go template expressions even though they resolve to a struct.
	for known := range knownTemplateVars {
		if strings.HasPrefix(known, v+".") {
			return true
		}
	}

	return false
}
