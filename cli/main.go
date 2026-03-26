package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "deploy":
		cmdDeploy(args)
	case "list":
		cmdList(args)
	case "deploys":
		cmdDeploys(args)
	case "rollback":
		cmdRollback(args)
	case "token":
		cmdToken(args)
	case "login":
		cmdLogin(args)
	case "logout":
		cmdLogout(args)
	case "config":
		cmdConfig(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: velori <command> [options]

Commands:
  deploy <path>                Deploy a folder or zip to Velori
  list                         List your projects
  deploys <name>               Show deploy history for a project
  rollback <name> <deploy-id>  Roll back to a previous deploy
  token                        Show current token info
  login                        Authenticate and save your API token
  logout                       Remove saved token
  config                       View and update CLI configuration

Configuration:
  Config file: ~/.velori/config.json
  Priority:    --flag > VELORI_TOKEN/VELORI_URL > config file > default`)
}

func cmdDeploy(args []string) {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	name := fs.String("name", "", "Project name (defaults to directory/zip name)")
	label := fs.String("label", "", "Deploy label (e.g. 'v2 with new header')")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	jsonOutput := fs.Bool("json", false, "Output JSON response")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: velori deploy <path> [--name NAME] [--token TOKEN] [--url URL] [--json]")
		os.Exit(1)
	}

	path := fs.Arg(0)
	client := newClient(requireToken(*token), resolveURL(*url))

	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	projectName := *name
	if projectName == "" {
		projectName = info.Name()
		if !info.IsDir() {
			projectName = stripZipExt(projectName)
		}
	}

	var result deployResult
	if info.IsDir() {
		result, err = client.deployDirectory(path, projectName, *label)
	} else {
		result, err = client.deployZip(path, projectName, *label)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		fmt.Println(result.raw)
	} else {
		fmt.Println(result.liveURL)
	}
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	client := newClient(requireToken(*token), resolveURL(*url))
	projects, err := client.listProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Fprintln(os.Stderr, "no projects")
		return
	}

	for _, p := range projects {
		fmt.Printf("%-24s %3d deploys  %s\n", p.Name, p.DeployCount, p.LiveURL)
	}
}

func cmdDeploys(args []string) {
	fs := flag.NewFlagSet("deploys", flag.ExitOnError)
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: velori deploys <project-name>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findProject(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	deploys, err := client.listDeploys(proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(deploys) == 0 {
		fmt.Fprintln(os.Stderr, "no deploys")
		return
	}

	for _, d := range deploys {
		current := "  "
		if d.IsCurrent {
			current = "* "
		}
		label := ""
		if d.Label != nil && *d.Label != "" {
			label = "  " + *d.Label
		}
		fmt.Printf("%s%-20s %4d files  %s%s\n", current, d.ID, d.FileCount, d.CreatedAt, label)
	}
}

func cmdRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: velori rollback <project-name> <deploy-id>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findProject(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	err = client.rollback(proj.ID, fs.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Rolled back %s to %s\n", proj.Name, fs.Arg(1))
}

func cmdToken(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	t := requireToken(*token)
	apiURL := resolveURL(*url)
	client := newClient(t, apiURL)
	user, err := client.getSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Token:    %s\n", maskToken(t))
	fmt.Printf("User:     %s (%s)\n", user.Name, user.Email)
	fmt.Printf("API:      %s\n", apiURL)
}

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("VELORI_TOKEN"); env != "" {
		return env
	}
	return loadConfig().Token
}

func resolveURL(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("VELORI_URL"); env != "" {
		return env
	}
	if cfg := loadConfig().URL; cfg != "" {
		return cfg
	}
	return "http://localhost:8080"
}

func maskToken(t string) string {
	if len(t) <= 12 {
		return t
	}
	return t[:8] + "..." + t[len(t)-4:]
}

func requireToken(flag string) string {
	t := resolveToken(flag)
	if t == "" {
		fmt.Fprintln(os.Stderr, `Error: no API token configured.

Set up authentication:
  velori login

Or provide a token directly:
  velori deploy --token vtk_your_token_here

Create tokens in the Velori dashboard under Settings > API Tokens.`)
		os.Exit(1)
	}
	return t
}

func cmdLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	token := fs.String("token", "", "API token")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	t := *token
	if t == "" {
		baseURL := strings.TrimRight(resolveURL(*url), "/")
		browserURL := baseURL + "?cli-auth"

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Press Enter to open your browser and log in.")
		reader.ReadString('\n')

		if err := openBrowser(browserURL); err != nil {
			fmt.Printf("Open this URL in your browser: %s\n\n", browserURL)
		} else {
			fmt.Println()
		}

		fmt.Print("Paste your API token: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
			os.Exit(1)
		}
		t = strings.TrimSpace(line)
	}

	if t == "" {
		fmt.Fprintln(os.Stderr, "no token provided")
		os.Exit(1)
	}

	client := newClient(t, resolveURL(*url))
	user, err := client.getSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg := loadConfig()
	cfg.Token = t
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nAuthenticated as %s (%s)\n", user.Name, user.Email)
	fmt.Printf("Token saved to %s\n", configPath())
	fmt.Println("\nYou're ready to deploy:")
	fmt.Println("  velori deploy ./my-site")
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	fs.Parse(args)

	cfg := loadConfig()
	cfg.Token = ""
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Token removed from", configPath())
}

func cmdConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `Usage: velori config <subcommand>

Subcommands:
  show               Show current configuration
  set token <value>  Save API token
  set url <value>    Save API base URL`)
		os.Exit(1)
	}

	switch args[0] {
	case "show":
		cfg := loadConfig()
		url := resolveURL("")
		fmt.Printf("Token:   %s\n", maskToken(cfg.Token))
		fmt.Printf("URL:     %s\n", url)
		fmt.Printf("Config:  %s\n", configPath())

	case "set":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: velori config set <key> <value>")
			os.Exit(1)
		}
		cfg := loadConfig()
		switch args[1] {
		case "token":
			cfg.Token = args[2]
		case "url":
			cfg.URL = args[2]
		default:
			fmt.Fprintf(os.Stderr, "unknown config key: %s\n", args[1])
			os.Exit(1)
		}
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Saved %s to %s\n", args[1], configPath())

	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func stripZipExt(name string) string {
	if len(name) > 4 && name[len(name)-4:] == ".zip" {
		return name[:len(name)-4]
	}
	return name
}
