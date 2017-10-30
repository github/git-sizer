package pipe

import (
	"bufio"
	"io"
	"os/exec"
	"sync"
)

type CommandPipe struct {
	command *exec.Cmd

	lock         sync.Mutex
	stdin        io.WriteCloser
	stdoutWriter io.ReadCloser
	stdout       *bufio.Reader
}

func NewCommandPipe(name string, arg ...string) (*CommandPipe, error) {
	command := exec.Command(name, arg...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	err = command.Start()
	if err != nil {
		return nil, err
	}
	return &CommandPipe{
		command:      command,
		stdin:        stdin,
		stdoutWriter: stdout,
		stdout:       bufio.NewReader(stdout),
	}, nil
}

func (p *CommandPipe) RunQuery(query string, handler func(*bufio.Reader)) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, err := io.WriteString(p.stdin, query)
	if err != nil {
		return err
	}
	handler(p.stdout)
	return nil
}

func (p *CommandPipe) Close() error {
	p.stdoutWriter.Close()
	p.stdin.Close()
	return p.command.Wait()
}
