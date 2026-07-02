package proto

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Registry provides protobuf schema parsing and resolution logic.
type Registry struct{}

// NewRegistry creates a new Registry instance.
func NewRegistry() *Registry {
	return &Registry{}
}

// ParseFileDescriptorSet validates binary input and returns a parsed FileDescriptorSet.
// Returns an error if the data is not a valid serialized FileDescriptorSet.
func (r *Registry) ParseFileDescriptorSet(data []byte) (*descriptorpb.FileDescriptorSet, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty FileDescriptorSet input")
	}

	fds := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, fds); err != nil {
		return nil, fmt.Errorf("invalid FileDescriptorSet binary: %w", err)
	}

	if len(fds.GetFile()) == 0 {
		return nil, fmt.Errorf("FileDescriptorSet contains no file descriptors")
	}

	return fds, nil
}

// ParseProtoFiles parses raw .proto file contents and returns a FileDescriptorSet.
// The files map keys are filenames (e.g. "service.proto") and values are file contents.
// Returns an error if imports are unresolved or files contain invalid syntax.
func (r *Registry) ParseProtoFiles(files map[string][]byte) (*descriptorpb.FileDescriptorSet, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no proto files provided")
	}

	// Build a resolver that serves provided files and falls back to well-known protos.
	resolver := &mapResolver{files: files}

	// Collect filenames in sorted order for deterministic compilation.
	filenames := make([]string, 0, len(files))
	for name := range files {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)

	// Track errors for reporting.
	var errs []string
	rep := reporter.NewReporter(
		func(err reporter.ErrorWithPos) error {
			errs = append(errs, err.Error())
			return nil // collect all errors, don't abort on first
		},
		nil, // no warning handler
	)

	compiler := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(resolver),
		Reporter:       rep,
		SourceInfoMode: protocompile.SourceInfoStandard,
	}

	compiled, err := compiler.Compile(context.Background(), filenames...)
	if err != nil {
		// Check for unresolved imports specifically.
		unresolvedImports := findUnresolvedImports(files, errs)
		if len(unresolvedImports) > 0 {
			return nil, fmt.Errorf("unresolved imports: %s", strings.Join(unresolvedImports, ", "))
		}
		// Return generic compilation error with collected details.
		if len(errs) > 0 {
			return nil, fmt.Errorf("proto compilation failed: %s", strings.Join(errs, "; "))
		}
		return nil, fmt.Errorf("proto compilation failed: %w", err)
	}

	// Build FileDescriptorSet from compiled results.
	// Each linker.File implements protoreflect.FileDescriptor, so we use
	// protodesc.ToFileDescriptorProto to convert back to descriptor proto form.
	fds := &descriptorpb.FileDescriptorSet{}
	seen := make(map[string]bool)

	for _, file := range compiled {
		collectDependencies(file, fds, seen)
	}

	return fds, nil
}

// collectDependencies recursively adds a file descriptor and its imports to the set.
func collectDependencies(fd protoreflect.FileDescriptor, fds *descriptorpb.FileDescriptorSet, seen map[string]bool) {
	name := string(fd.Path())
	if seen[name] {
		return
	}
	seen[name] = true

	// Add dependencies first so they appear before dependents in the set.
	imports := fd.Imports()
	for i := range imports.Len() {
		dep := imports.Get(i).FileDescriptor
		if dep != nil {
			collectDependencies(dep, fds, seen)
		}
	}

	// Convert the protoreflect.FileDescriptor back to its proto representation.
	fdp := protodesc.ToFileDescriptorProto(fd)
	fds.File = append(fds.File, fdp)
}

// findUnresolvedImports extracts unresolved import paths from compilation errors.
func findUnresolvedImports(files map[string][]byte, errs []string) []string {
	var unresolved []string
	seen := make(map[string]bool)

	for _, errMsg := range errs {
		lower := strings.ToLower(errMsg)
		if strings.Contains(lower, "import") || strings.Contains(lower, "not found") || strings.Contains(lower, "could not resolve") {
			path := extractQuotedPath(errMsg)
			if path != "" && !seen[path] {
				if _, provided := files[path]; !provided {
					seen[path] = true
					unresolved = append(unresolved, path)
				}
			}
		}
	}

	return unresolved
}

// extractQuotedPath finds the first quoted string in an error message.
func extractQuotedPath(msg string) string {
	if idx := strings.Index(msg, "\""); idx >= 0 {
		rest := msg[idx+1:]
		if end := strings.Index(rest, "\""); end >= 0 {
			return rest[:end]
		}
	}
	return ""
}
