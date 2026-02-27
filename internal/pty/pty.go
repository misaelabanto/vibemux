package pty

import (
	"os"
	"os/exec"

	gospty "github.com/creack/pty"
)

type Pty struct {
	f   *os.File
	cmd *exec.Cmd
}

func Start(dir string, cols, rows int) (*Pty, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Dir = dir
	f, err := gospty.StartWithSize(cmd, &gospty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	return &Pty{f: f, cmd: cmd}, nil
}

func (p *Pty) Read(buf []byte) (int, error) {
	return p.f.Read(buf)
}

func (p *Pty) Write(data []byte) {
	p.f.Write(data)
}

func (p *Pty) Resize(cols, rows int) {
	gospty.Setsize(p.f, &gospty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (p *Pty) Close() {
	p.f.Close()
	if p.cmd.Process != nil {
		p.cmd.Process.Kill()
	}
}
