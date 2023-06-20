package hwaas

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/linuxboot/contest/pkg/event"
	"github.com/linuxboot/contest/pkg/event/testevent"
	"github.com/linuxboot/contest/pkg/target"
	"github.com/linuxboot/contest/pkg/xcontext"
)

// events that we may emit during the plugin's lifecycle
const (
	EventStdout = event.Name("Stdout")
	EventStderr = event.Name("Stderr")
)

// Events defines the events that a TestStep is allow to emit. Emitting an event
// that is not registered here will cause the plugin to terminate with an error.
var Events = []event.Name{
	EventStdout,
	EventStderr,
}

type eventPayload struct {
	Msg string
}

func emitEvent(ctx xcontext.Context, name event.Name, payload interface{}, tgt *target.Target, ev testevent.Emitter) error {
	payloadData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot marshal payload for event '%s': %w", name, err)
	}

	msg := json.RawMessage(payloadData)
	data := testevent.Data{
		EventName: name,
		Target:    tgt,
		Payload:   &msg,
	}

	if err := ev.Emit(ctx, data); err != nil {
		return fmt.Errorf("cannot emit event EventCmdStart: %w", err)
	}

	return nil
}

// Function to format teststep information and append it to a string builder.
func writeTestStep(builder *strings.Builder, step *TestStep) {
	builder.WriteString("Input Parameter:\n")
	builder.WriteString("  Parameter:\n")
	builder.WriteString(fmt.Sprintf("    Host: %s\n", step.Parameter.Host))
	builder.WriteString(fmt.Sprintf("    Port: %d\n", step.Parameter.Port))
	builder.WriteString(fmt.Sprintf("    Command: %s\n", step.Parameter.Command))
	builder.WriteString(fmt.Sprintf("    Arguments: %s\n", step.Parameter.Args))
	builder.WriteString("  Options:\n")
	builder.WriteString(fmt.Sprintf("    Timeout: %s\n", time.Duration(step.Options.Timeout)))
	builder.WriteString("\n")

	builder.WriteString("Default Values:\n")
	builder.WriteString(fmt.Sprintf("  Port: %d\n", defaultPort))
	builder.WriteString(fmt.Sprintf("  Timeout: %s\n", defaultTimeout))

	builder.WriteString("\n\n")
}

// Function to format command information and append it to a string builder.
func writeCommand(builder *strings.Builder, command string, args ...string) {
	builder.WriteString("Operation on DUT:\n")
	builder.WriteString(fmt.Sprintf("%s %s", command, strings.Join(args, " ")))
	builder.WriteString("\n\n")

	builder.WriteString("Output:\n")
}

// Function to format command output information and append it to a string builder.
func writeCommandOutput(builder *strings.Builder, stdout string) {
	builder.WriteString(stdout)
	builder.WriteString("\n")
}