package cli

import (
	"context"
	"fmt"

	"mobius/pkg/session"
)

func RunGoal(app *App, s *session.Session, userInstruction string) error {
	if !s.Started() {
		s.MarkStarted(s.Name())
	}
	ctx := context.Background()
	res, err := app.DispatchUserInput(ctx, s, userInstruction, true)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	if res.Reply != "" {
		fmt.Printf("%s\n", res.Reply)
	}
	return nil
}

// RunGoalAsync prints the immediate orchestration reply without waiting on workers.
func RunGoalAsync(app *App, s *session.Session, userInstruction string) error {
	if !s.Started() {
		s.MarkStarted(s.Name())
	}
	ctx := context.Background()
	res, err := app.DispatchUserInput(ctx, s, userInstruction, false)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return err
	}
	if res.Reply != "" {
		fmt.Printf("%s\n", res.Reply)
	}
	return nil
}
