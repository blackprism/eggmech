package main

import (
	"context"

	"dagger/eggmech/internal/dagger"
)

type Eggmech struct{}

func (e *Eggmech) Linter(
	ctx context.Context,
// configuration file of golangci-lint
	config *dagger.File,

// application directory source
	applicationDir *dagger.Directory,

	directory string,

// fix files when its possible
// +optional
	fix bool,
) *dagger.Container {
	entrypoint := []string{"golangci-lint", "run", "-c", "/.golangci.yaml", "./..."}

	if fix {
		entrypoint = append(entrypoint, "--fix", "--issues-exit-code", "0")
	}

	return dag.Container().
		From("golangci/golangci-lint:v1.62.2").
		WithMountedDirectory("/app", applicationDir).
		WithFile("/.golangci.yaml", config).
		WithWorkdir("/app/" + directory).
		WithExec(entrypoint)
}

func (e *Eggmech) Vet(ctx context.Context, applicationDir *dagger.Directory) (string, error) {
	return dag.Container().
		From("golang:1.22").
		WithMountedDirectory("/app", applicationDir).
		WithWorkdir("/app").
		WithEntrypoint([]string{"go", "vet", "./..."}).
		Stdout(ctx)
}

func (e *Eggmech) Staticcheck(ctx context.Context, applicationDir *dagger.Directory) (string, error) {
	return dag.Container().
		From("golang:1.22").
		WithMountedDirectory("/app", applicationDir).
		WithWorkdir("/app").
		WithExec([]string{"go", "install", "honnef.co/go/tools/cmd/staticcheck@latest"}).
		WithExec([]string{"staticcheck", "./..."}).
		Stdout(ctx)
}
