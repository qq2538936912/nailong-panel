//go:build windows

package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type terminalStdioPipe struct {
	in  io.WriteCloser
	out io.ReadCloser
}

func (p *terminalStdioPipe) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *terminalStdioPipe) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *terminalStdioPipe) Close() error {
	errIn := p.in.Close()
	errOut := p.out.Close()
	if errIn != nil {
		return errIn
	}
	return errOut
}

func startTerminalProcess(shell, workDir string, env []string, _, _ uint16) (*exec.Cmd, io.ReadWriteCloser, func(cols, rows uint16) error, error) {
	cmd := exec.Command(shell)
	cmd.Dir = workDir
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("启动终端失败: %w", err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, fmt.Errorf("启动终端失败: %w", err)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, nil, fmt.Errorf("启动终端失败: %w", err)
	}
	_ = writer.Close()

	resize := func(uint16, uint16) error { return nil }
	return cmd, &terminalStdioPipe{in: stdin, out: reader}, resize, nil
}
