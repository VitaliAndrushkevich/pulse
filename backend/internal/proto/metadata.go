package proto

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ExtractMetadata walks a FileDescriptorSet and extracts all services with their methods,
// input/output types, and collects all top-level message type names.
func ExtractMetadata(fds *descriptorpb.FileDescriptorSet) (*ProtoSourceMetadata, error) {
	if fds == nil {
		return nil, fmt.Errorf("FileDescriptorSet is nil")
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, fmt.Errorf("failed to create file registry: %w", err)
	}

	meta := &ProtoSourceMetadata{}

	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		meta.Filenames = append(meta.Filenames, fd.Path())

		// Extract services and their methods.
		for i := range fd.Services().Len() {
			sd := fd.Services().Get(i)
			svc := ProtoService{
				FullName: string(sd.FullName()),
			}

			for j := range sd.Methods().Len() {
				md := sd.Methods().Get(j)
				method := ProtoMethod{
					Name:       string(md.Name()),
					FullName:   string(sd.FullName()) + "/" + string(md.Name()),
					InputType:  string(md.Input().FullName()),
					OutputType: string(md.Output().FullName()),
				}
				svc.Methods = append(svc.Methods, method)
			}

			meta.Services = append(meta.Services, svc)
		}

		// Extract top-level message types.
		for i := range fd.Messages().Len() {
			msg := fd.Messages().Get(i)
			meta.MessageTypes = append(meta.MessageTypes, string(msg.FullName()))
		}

		return true
	})

	return meta, nil
}

// ResolveMessageDescriptor finds a message descriptor by fully-qualified name
// within the given FileDescriptorSet.
func ResolveMessageDescriptor(fds *descriptorpb.FileDescriptorSet, fullName string) (protoreflect.MessageDescriptor, error) {
	if fds == nil {
		return nil, fmt.Errorf("FileDescriptorSet is nil")
	}
	if fullName == "" {
		return nil, fmt.Errorf("message name is empty")
	}

	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, fmt.Errorf("failed to create file registry: %w", err)
	}

	desc, err := files.FindDescriptorByName(protoreflect.FullName(fullName))
	if err != nil {
		return nil, fmt.Errorf("message %q not found: %w", fullName, err)
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a message type (found %T)", fullName, desc)
	}

	return msgDesc, nil
}
