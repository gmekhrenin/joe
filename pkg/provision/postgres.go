/*
2019 © Postgres.ai
*/

package provision

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"time"
)

// TODO(anatoly): Use SQL runner.
// Use `RunPsqlStrict` for commands defined by a user!
func runPsql(r Runner, command string, useFile bool) (string, error) {
	var filename string
	commandParam := fmt.Sprintf(`-c "%s"`, command)
	if useFile {
		source := rand.NewSource(time.Now().UnixNano())
		random := rand.New(source)
		uid := random.Uint64()

		filename := fmt.Sprintf("/tmp/psql-query-%d", uid)

		err := ioutil.WriteFile(filename, []byte(command), 0644)
		if err != nil {
			return "", err
		}

		commandParam = fmt.Sprintf(`-f %s`, filename)
	}

	out, err := r.Run(commandParam)

	if useFile {
		os.Remove(filename)
	}

	return out, err
}

// Use for user defined commands to DB. Currently we only need
// to support limited number of PSQL meta information commands.
// That's why it's ok to restrict usage of some symbols.
func RunPsqlStrict(r Runner, command string) (string, error) {
	command = strings.Trim(command, " \n")
	if len(command) == 0 {
		return "", fmt.Errorf("Empty command")
	}

	// Psql file option (-f) allows to run any number of commands.
	// We need to take measures to restrict multiple commands support,
	// as we only check the first command.

	// User can run backslash commands on the same line with the first
	// backslash command (even without space separator),
	// e.g. `\d table1\d table2`.

	// Remove all backslashes except the one in the beggining.
	command = string(command[0]) + strings.ReplaceAll(command[1:], "\\", "")

	// Semicolumn creates possibility to run consequent command.
	command = strings.ReplaceAll(command, ";", "")

	// User can run any command (including DML queries) on other lines.
	// Restricting usage of multiline commands.
	command = strings.ReplaceAll(command, "\n", "")

	out, err := runPsql(r, command, true)
	if err != nil {
		if rerr, ok := err.(RunnerError); ok {
			return "", fmt.Errorf("Pqsl error: %s", rerr.Stderr)
		}

		return "", fmt.Errorf("Psql error")
	}

	return out, nil
}
