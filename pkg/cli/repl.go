package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mobius/pkg/guides"
	"mobius/pkg/orchestrator"
	"mobius/pkg/sensors"

	"github.com/charmbracelet/x/term"
)

func readPrompt(prompt string) (string, bool) {
	fmt.Print(prompt)
	fd := os.Stdin.Fd()
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", false
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	var input []rune
	buf := make([]byte, 1)

	for {
		_, err = os.Stdin.Read(buf)
		if err != nil {
			return "", false
		}

		b := buf[0]

		// Enter key
		if b == '\r' || b == '\n' {
			fmt.Print("\r\n")
			return string(input), true
		}

		// Ctrl+C (ASCII 3)
		if b == 3 {
			fmt.Print("\r\n")
			return "", false
		}

		// Escape key (ASCII 27)
		if b == 27 {
			// If it's a standalone Esc, clear current input
			for len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}

		// Slash menu trigger on empty line
		if b == '/' && len(input) == 0 {
			_ = term.Restore(fd, oldState)
			fmt.Print("\r\n")

			selected := ShowActionMenu()
			if selected != "" {
				return selected, true
			}

			oldState, _ = term.MakeRaw(fd)
			fmt.Print(prompt)
			continue
		}

		// Backspace (127 / 8)
		if b == 127 || b == 8 {
			if len(input) > 0 {
				input = input[:len(input)-1]
				fmt.Print("\b \b")
			}
			continue
		}

		// Printable characters
		if b >= 32 && b <= 126 {
			r := rune(b)
			input = append(input, r)
			fmt.Print(string(r))
		}
	}
}

func StartREPL(app *App) {
	PrintBanner(app.Config.ActiveModel)
	sm := app.Sessions

	for {
		input, ok := readPrompt("mobius > ")
		if !ok {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		switch input {
		case "/exit":
			fmt.Println("Goodbye!")
			return
		case "/listchats":
			selectedID := ShowSessionMenu(sm)
			if selectedID != "" {
				s, err := sm.SwitchSession(selectedID)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				} else {
					fmt.Printf("Switched to session: '%s'\n", s.Name())
				}
			}
		case "/newchat":
			handleNewChat(app)
		case "/models":
			activeSession, err := sm.GetActive()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			currentModel := activeSession.Agent.GetModel()
			model, err := ShowModelMenu(app.Config, currentModel)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			if model == "" || model == currentModel {
				continue // User cancelled with Esc or re-selected current model
			}
			provider, err := app.Config.GetProviderForModel(model)
			if err != nil {
				fmt.Printf("Error getting provider for %s: %v\n", model, err)
				continue
			}
			pCost, cCost := app.Config.GetPrices(model)
			activeSession.Agent.SetModel(model, provider, pCost, cCost)
			if ws, err := app.WorkspaceForSession(activeSession); err == nil {
				ws.SetModel(orchestrator.ModelSpec{
					Name:            model,
					Provider:        provider,
					PromptPrice:     pCost,
					CompletionPrice: cCost,
				})
			}
			fmt.Printf("Switched model to '%s'\n", model)
		case "/init":
			activeSession, err := sm.GetActive()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			provider, err := app.Config.GetProviderForModel(activeSession.Agent.GetModel())
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			fmt.Printf("Scanning workspace and generating verification commands with %s...\n", activeSession.Agent.GetModel())
			gen, err := guides.InitWorkspace(context.Background(), provider, activeSession.Agent.GetModel(), ".")
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			if agentsData, err := os.ReadFile("AGENTS.md"); err == nil {
				activeSession.Agent.SetSensors(sensors.NewRegistryFromGuide(".", string(agentsData)))
			}
			fmt.Printf("\nAGENTS.md updated successfully:\n\n%s\n", gen)
		default:
			// Normal AI Prompt
			activeSession, err := sm.GetActive()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}
			_ = RunGoalAsync(app, activeSession, input)
		}
	}
}

// Helper to keep StartREPL tidy
func handleNewChat(app *App) {
	sm := app.Sessions
	if unstarted := sm.GetUnstarted(); unstarted != nil {
		_, _ = sm.SwitchSession(unstarted.ID)
		fmt.Printf("Reusing empty session '%s' (ID: %s).\n", unstarted.Name(), unstarted.ID)
		return
	}

	provider, err := app.Config.GetProviderForModel("")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	spec := app.ActiveModelSpec(provider)
	ws, err := orchestrator.NewWorkspace(context.Background(), "new-chat", app.RoleStore, app.BaseTools, spec, app.AgentFactory())
	if err != nil {
		fmt.Printf("Error creating workspace: %v\n", err)
		return
	}
	orch := ws.OrchestratorSession()
	if agentsData, err := os.ReadFile("AGENTS.md"); err == nil {
		orch.Agent.SetSensors(sensors.NewRegistryFromGuide(".", string(agentsData)))
	}
	newSess := sm.CreateSession("New Chat", orch.Agent)
	newSess.MarkStarted("New Chat")
	app.Workspaces.Register(newSess.ID, ws)
	fmt.Printf("Started new session '%s' (ID: %s)\n", newSess.Name(), newSess.ID)
}
