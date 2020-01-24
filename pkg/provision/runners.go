/*
2019 © Postgres.ai
*/

package provision

import (
	"bytes"
	"fmt"
	"gitlab.com/postgres-ai/joe/pkg/bot"
	"os/exec"
	"strings"
	"syscall"

	"gitlab.com/postgres-ai/database-lab/pkg/log"
)

const (
	LogsEnabledDefault = true
	HIDDEN             = "HIDDEN"
)

type Runner interface {
	Run(command string) (output string, err error)
}

type RunnerError struct {
	Msg        string
	ExitStatus int
	Stderr     string
}

func NewRunnerError(command string, stderr string, e error) error {
	exitStatus := 0

	switch err := e.(type) {
	case RunnerError:
		return err

	case (*exec.ExitError):
		// SO: https://stackoverflow.com/questions/10385551/get-exit-code-go.
		// The program has exited with an exit code != 0

		// This works on both Unix and Windows. Although package
		// syscall is generally platform dependent, WaitStatus is
		// defined for both Unix and Windows and in both cases has
		// an ExitStatus() method with the same signature.

		if status, ok := err.Sys().(syscall.WaitStatus); ok {
			exitStatus = status.ExitStatus()
		}
	}

	msg := fmt.Sprintf(`RunnerError(cmd="%s", inerr="%v", stderr="%s" exit="%d")`,
		command, e, stderr, exitStatus)

	return RunnerError{
		Msg:        msg,
		ExitStatus: exitStatus,
		Stderr:     stderr,
	}
}

func (e RunnerError) Error() string {
	return e.Msg
}

// SQL.
// TODO(anatoly): Use in ProvisionAws, Postgres functions.
type SQLRunner struct {
	logEnabled bool
	clone      bot.DBLabClone
}

func NewSQLRunner(clone bot.DBLabClone, logEnabled bool) *SQLRunner {
	return &SQLRunner{
		clone:      clone,
		logEnabled: logEnabled,
	}
}

func (r *SQLRunner) Run(commandParam string) (string, error) {
	log.Dbg(fmt.Sprintf(`SQLRun: "%s"`, commandParam))

	var out bytes.Buffer
	var stderr bytes.Buffer

	cmdStr := fmt.Sprintf("PGPASSWORD=%s PGSSLMODE=%s psql --host=%s --port=%s --dbname=%q -X %s",
		r.clone.Password, r.clone.SSLMode, r.clone.Host, r.clone.Port, r.clone.Username, r.clone.Name, commandParam)

	cmd := exec.Command("/bin/bash", "-c", cmdStr)

	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Dbg(cmd.String())

	// Psql with the file option returns error reponse to stderr with
	// success exit code. In that case err will be nil, but we need
	// to treat the case as error and read proper output.
	err := cmd.Run()

	if err != nil || stderr.String() != "" {
		runnerError := NewRunnerError(commandParam, stderr.String(), err)

		log.Err(runnerError)
		return "", runnerError
	}

	outFormatted := strings.Trim(out.String(), " \n")

	logOut := HIDDEN
	if r.logEnabled {
		logOut = outFormatted
	}

	log.Dbg(fmt.Sprintf(`SQLRun: output "%s"`, logOut))

	return outFormatted, nil
}
