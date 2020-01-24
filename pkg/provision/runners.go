/*
2019 © Postgres.ai
*/

package provision

import (
	"bytes"
	"fmt"
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

//// Local.
//type LocalRunner struct {
//}
//
//func NewLocalRunner() *LocalRunner {
//	r := &LocalRunner{}
//
//	return r
//}
//
//func (r *LocalRunner) Run(command string, options ...bool) (string, error) {
//	command = strings.Trim(command, " \n")
//	if len(command) == 0 {
//		return "", fmt.Errorf("Empty command")
//	}
//
//	logsEnabled := parseOptions(options...)
//
//	logCommand := HIDDEN
//	if logsEnabled {
//		logCommand = command
//	}
//
//	log.Dbg(fmt.Sprintf(`Run(Local): "%s"`, logCommand))
//
//	var out bytes.Buffer
//	var stderr bytes.Buffer
//
//	if runtime.GOOS == "windows" {
//		return "", fmt.Errorf("Windows is not supported")
//	}
//
//	cmd := exec.Command("/bin/bash", "-c", command)
//
//	cmd.Stdout = &out
//	cmd.Stderr = &stderr
//
//	// Psql with the file option returns error reponse to stderr with
//	// success exit code. In that case err will be nil, but we need
//	// to treat the case as error and read proper output.
//	err := cmd.Run()
//
//	if err != nil || stderr.String() != "" {
//		runnerError := NewRunnerError(logCommand, stderr.String(), err)
//
//		log.Err(runnerError)
//		return "", runnerError
//	}
//
//	outFormatted := strings.Trim(out.String(), " \n")
//
//	logOut := HIDDEN
//	if logsEnabled {
//		logOut = outFormatted
//	}
//
//	log.Dbg(fmt.Sprintf(`Run(Local): output "%s"`, logOut))
//
//	return outFormatted, nil
//}

// SQL.
// TODO(anatoly): Use in ProvisionAws, Postgres functions.
type SQLRunner struct {
	connectionString string
	password         string
	logEnabled       bool
}

func NewSqlRunner(connectionString, password string, logEnabled bool) *SQLRunner {
	return &SQLRunner{
		connectionString: connectionString,
		password:         password,
		logEnabled:       logEnabled,
	}
}

//func (r *SQLRunner) Run(command string, options ...bool) (string, error) {
//	cmd := fmt.Sprintf(`psql -U postgres -t -c "%s"`, command)
//	return r.InnerRunner.Run(cmd, options...)
//}

func (r *SQLRunner) Run(commandParam string) (string, error) {
	//command = strings.Trim(command, " \n")
	//if len(command) == 0 {
	//	return "", fmt.Errorf("Empty command")
	//}

	//logsEnabled := parseOptions(options...)

	//logCommand := HIDDEN
	//if logsEnabled {
	//	logCommand = command
	//}

	log.Dbg(fmt.Sprintf(`SQLRun: "%s"`, commandParam))

	var out bytes.Buffer
	var stderr bytes.Buffer

	psqlCmd := fmt.Sprintf("%q -X %s", r.connectionString, commandParam)
	cmd := exec.Command("psql", "-c", psqlCmd)

	cmd.Env = append(cmd.Env, "PGPASSWORD="+r.password)
	cmd.Stdout = &out
	cmd.Stderr = &stderr

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
