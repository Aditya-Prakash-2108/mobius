package main

import (
	"context"
	"fmt"
	"mobius/pkg/agent"
	"mobius/pkg/cli"
	"mobius/pkg/events"
	"mobius/pkg/guides"
	"mobius/pkg/llm"
	"mobius/pkg/orchestrator"
	"mobius/pkg/roles"
	"mobius/pkg/sensors"
	"mobius/pkg/session"
	"mobius/pkg/tools"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	registry := tools.NewDefaultRegistry(".")

	eventStore, err := events.NewFileEventStore(".mobius/events", 256)
	if err != nil {
		fmt.Printf("Error: Failed to init event store: %v\n", err)
		os.Exit(1)
	}
	defer eventStore.Close()

	cfg, err := llm.LoadConfig("config/model_config.toml")
	if err != nil {
		cfg, err = llm.LoadConfig("model_config.toml")
		if err != nil {
			fmt.Printf("Error: Failed to load config: %v\n", err)
			os.Exit(1)
		}
	}

	roleStore, err := roles.Open(context.Background(), "roles")
	if err != nil {
		fmt.Printf("Error: Failed to load roles: %v\n", err)
		os.Exit(1)
	}

	provider, err := cfg.GetProviderForModel("")
	if err != nil {
		fmt.Printf("Error: Failed to get provider: %v\n", err)
		os.Exit(1)
	}

	pCost, cCost := cfg.GetPrices(cfg.ActiveModel)
	model := orchestrator.ModelSpec{
		Name:            cfg.ActiveModel,
		Provider:        provider,
		PromptPrice:     pCost,
		CompletionPrice: cCost,
	}

	factory := func(reg *tools.Registry, m orchestrator.ModelSpec) (*agent.Agent, error) {
		return agent.NewAgent(m.Provider, reg, m.Name, m.PromptPrice, m.CompletionPrice, eventStore)
	}

	ctx := context.Background()
	defaultWS, err := orchestrator.NewWorkspace(ctx, "default", roleStore, registry, model, factory)
	if err != nil {
		fmt.Printf("Error: Failed to create workspace: %v\n", err)
		os.Exit(1)
	}

	defaultSess := defaultWS.OrchestratorSession()
	if agentsData, err := os.ReadFile("AGENTS.md"); err == nil {
		defaultSess.Agent.SetSensors(sensors.NewRegistryFromGuide(".", string(agentsData)))
	}
	defaultSess.MarkStarted("default")
	manager := session.NewManager(defaultSess)

	wsReg := orchestrator.NewWorkspaceRegistry()
	wsReg.Register(defaultSess.ID, defaultWS)

	app := cli.NewApp(manager, wsReg, roleStore, registry, cfg, eventStore)

	if len(os.Args) > 1 {
		if os.Args[1] == "init" {
			targetDir := "."
			if len(os.Args) > 2 {
				targetDir = os.Args[2]
			}
			fmt.Printf("Scanning workspace (%s) and generating verification commands with %s...\n", targetDir, cfg.ActiveModel)
			gen, err := guides.InitWorkspace(context.Background(), provider, cfg.ActiveModel, targetDir)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("\nAGENTS.md updated successfully:\n\n%s\n", gen)
			return
		}

		userInstruction := strings.Join(os.Args[1:], " ")
		if err := cli.RunGoal(app, defaultSess, userInstruction); err != nil {
			os.Exit(1)
		}
		defaultWS.Close()
		return
	}

	cli.StartInteractive(app)
}
