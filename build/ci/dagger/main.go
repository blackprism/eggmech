package main

import (
	"context"
)

type Eggmech struct{}

func (e *Eggmech) Linter(
	ctx context.Context,
	// configuration file of golangci-lint
	config *File,

	// application directory source
	applicationDir *Directory,

	directory string,

	// fix files when its possible
	// +optional
	fix bool,
) *Container {
	entrypoint := []string{"golangci-lint", "run", "-c", "/.golangci.yaml", "./..."}

	if fix {
		entrypoint = append(entrypoint, "--fix", "--issues-exit-code", "0")
	}

	return dag.Container().
		From("golangci/golangci-lint:v1.58.1").
		WithMountedDirectory("/app", applicationDir).
		WithFile("/.golangci.yaml", config).
		WithWorkdir("/app/" + directory).
		WithExec(entrypoint)
}

func (e *Eggmech) Vet(ctx context.Context, applicationDir *Directory) (string, error) {
	return dag.Container().
		From("golang:1.22").
		WithMountedDirectory("/app", applicationDir).
		WithWorkdir("/app").
		WithEntrypoint([]string{"go", "vet", "./..."}).
		Stdout(ctx)
}

func (e *Eggmech) Staticcheck(ctx context.Context, applicationDir *Directory) (string, error) {
	return dag.Container().
		From("golang:1.22").
		WithMountedDirectory("/app", applicationDir).
		WithWorkdir("/app").
		WithExec([]string{"go", "install", "honnef.co/go/tools/cmd/staticcheck@latest"}).
		WithExec([]string{"staticcheck", "./..."}).
		Stdout(ctx)
}
