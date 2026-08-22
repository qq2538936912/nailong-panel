//go:build !windows

package service

import (
	"fmt"
	"io"
	"os/exec"

	"github.com/creack/pty"
)

func startTerminalProcess(shell, workDir string, env []string, cols, rows uint16) (*exec.Cmd, io.ReadWriteCloser, func(cols, rows uint16) error, error) {
	cmd := exec.Command(shell)
	cmd.Dir = workDir
	cmd.Env = env

	ptyFile, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("启动终端失败: %w", err)
	}

	resize := func(nextCols, nextRows uint16) error {
		return pty.Setsize(ptyFile, &pty.Winsize{Cols: nextCols, Rows: nextRows})
	}
	return cmd, ptyFile, resize, nil
}
