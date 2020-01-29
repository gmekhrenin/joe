/*
2019 © Postgres.ai
*/

package transmission

type Runner interface {
	Run(command string) (output string, err error)
}
