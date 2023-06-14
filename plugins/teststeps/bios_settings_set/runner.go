package bios_settings_set

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/linuxboot/contest/pkg/event/testevent"
	"github.com/linuxboot/contest/pkg/target"
	"github.com/linuxboot/contest/pkg/test"
	"github.com/linuxboot/contest/pkg/xcontext"
	"github.com/linuxboot/contest/plugins/teststeps/abstraction/transport"
)

const (
	supportedProto = "ssh"
	privileged     = "sudo"
	cmd            = "wmi"
	jsonFlag       = "--json"
)

type outcome error

type Error struct {
	Msg string `json:"error"`
}

type TargetRunner struct {
	ts *TestStep
	ev testevent.Emitter
}

func NewTargetRunner(ts *TestStep, ev testevent.Emitter) *TargetRunner {
	return &TargetRunner{
		ts: ts,
		ev: ev,
	}
}

func (r *TargetRunner) Run(ctx xcontext.Context, target *target.Target) error {
	ctx.Infof("Executing on target %s", target)

	// limit the execution time if specified
	timeout := r.ts.Options.Timeout
	if timeout != 0 {
		var cancel xcontext.CancelFunc
		ctx, cancel = xcontext.WithTimeout(ctx, time.Duration(timeout))
		defer cancel()
	}

	pe := test.NewParamExpander(target)

	var params inputStepParams
	if err := pe.ExpandObject(r.ts.inputStepParams, &params); err != nil {
		return err
	}

	if params.Transport.Proto != supportedProto {
		return fmt.Errorf("only %q is supported as protocol in this teststep", supportedProto)
	}

	if params.Parameter.Password == "" && params.Parameter.KeyPath == "" {
		return fmt.Errorf("password or certificate file must be set")
	}

	if params.Parameter.Option == "" || params.Parameter.Value == "" {
		return fmt.Errorf("bios option and value must be set")
	}

	transportProto, err := transport.NewTransport(params.Transport.Proto, params.Transport.Options, pe)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}

	// for any ambiguity, outcome is an error interface, but it encodes whether the process
	// was launched sucessfully and it resulted in a failure; err means the launch failed
	outcome, err := r.runSet(ctx, target, transportProto, params)
	if err != nil {
		return err
	}

	return outcome
}

func (r *TargetRunner) runSet(
	ctx xcontext.Context, target *target.Target,
	transport transport.Transport, params inputStepParams,
) (outcome, error) {
	var authString string
	if params.Parameter.Password != "" {
		authString = fmt.Sprintf("--password=%s", params.Parameter.Password)
	} else if params.Parameter.KeyPath != "" {
		authString = fmt.Sprintf("--private-key=%s", params.Parameter.KeyPath)
	}

	proc, err := transport.NewProcess(
		ctx,
		privileged,
		[]string{
			params.Parameter.ToolPath,
			cmd,
			"set",
			fmt.Sprintf("--option=%s", params.Parameter.Option),
			fmt.Sprintf("--value=%s", params.Parameter.Value),
			authString,
			jsonFlag,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create process: %v", err)
	}

	stdoutPipe, err := proc.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to pipe stdout: %v", err)
	}

	stderrPipe, err := proc.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to pipe stderr: %v", err)
	}

	// try to start the process, if that succeeds then the outcome is the result of
	// waiting on the process for its result; this way there's a semantic difference
	// between "an error occured while launching" and "this was the outcome of the execution"
	outcome := proc.Start(ctx)
	if outcome == nil {
		outcome = proc.Wait(ctx)
	}

	stdout, stderr := getOutputFromReader(stdoutPipe, stderrPipe)

	if err := parseSetOutput(stderr, params.Parameter.ShallFail); err != nil {
		return nil, err
	}

	if err := emitEvent(ctx, EventStdout, eventPayload{Msg: string(stdout)}, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %v", err)
	}
	if err := emitEvent(ctx, EventStderr, eventPayload{Msg: string(stderr)}, target, r.ev); err != nil {
		return nil, fmt.Errorf("cannot emit event: %v", err)
	}

	return outcome, nil
}

// getOutputFromReader reads data from the provided io.Reader instances
// representing stdout and stderr, and returns the collected output as byte slices.
func getOutputFromReader(stdout, stderr io.Reader) ([]byte, []byte) {
	// Read from the stdout and stderr pipe readers
	outBuffer, err := readBuffer(stdout)
	if err != nil {
		fmt.Printf("failed to read from Stdout buffer: %v\n", err)
	}

	errBuffer, err := readBuffer(stderr)
	if err != nil {
		fmt.Printf("failed to read from Stderr buffer: %v\n", err)
	}

	return outBuffer, errBuffer
}

// readBuffer reads data from the provided io.Reader and returns it as a byte slice.
// It dynamically accumulates the data using a bytes.Buffer.
func readBuffer(r io.Reader) ([]byte, error) {
	buf := &bytes.Buffer{}
	_, err := io.Copy(buf, r)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseSetOutput(stderr []byte, fail bool) error {
	err := Error{}

	if len(stderr) != 0 {
		if err := json.Unmarshal(stderr, &err); err != nil {
			return fmt.Errorf("failed to unmarshal stderr: %v", err)
		}
	}

	if err.Msg != "" {
		if err.Msg == "BIOS options are locked, needs unlocking." && fail {
			return nil
		} else if err.Msg != "" {
			return fmt.Errorf("%s", err.Msg)
		}
	}

	return nil
}