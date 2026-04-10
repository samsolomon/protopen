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

var version = "dev"

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
	case "visibility":
		cmdVisibility(args)
	case "login":
		cmdLogin(args)
	case "logout":
		cmdLogout(args)
	case "config":
		cmdConfig(args)
	case "version", "--version", "-v":
		fmt.Println(version)
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
  visibility <name> <public|private>  Set project visibility
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
	private := fs.Bool("private", false, "Make the project private after deploy")
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	jsonOutput := fs.Bool("json", false, "Output JSON response")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: velori deploy <path> [--name NAME] [--private] [--token TOKEN] [--url URL] [--json]")
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

	resolvedOrg := resolveOrg(*org)
	git := detectGitMeta(path)

	var result deployResult
	if info.IsDir() {
		result, err = client.deployDirectory(path, projectName, *label, resolvedOrg, git)
	} else {
		result, err = client.deployZip(path, projectName, *label, resolvedOrg, git)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *private {
		if err := client.updateVisibility(result.projectID, false); err != nil {
			fmt.Fprintf(os.Stderr, "warning: deployed but could not set private: %v\n", err)
		}
	}

	if *jsonOutput {
		fmt.Println(result.raw)
	} else {
		fmt.Println(result.liveURL)
		if git != nil {
			dirty := ""
			if git.Dirty {
				dirty = " (dirty)"
			}
			branch := git.Branch
			if branch == "" {
				branch = "detached"
			}
			fmt.Fprintf(os.Stderr, "  git: %s %s%s\n", branch, git.CommitHash[:8], dirty)
		}
	}
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	client := newClient(requireToken(*token), resolveURL(*url))
	projects, err := client.listProjects(resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Fprintln(os.Stderr, "no projects")
		return
	}

	for _, p := range projects {
		vis := "public"
		if !p.IsPublic {
			vis = "private"
		}
		fmt.Printf("%-24s %3d deploys  %-7s  %s\n", p.Name, p.DeployCount, vis, p.LiveURL)
	}
}

func cmdDeploys(args []string) {
	fs := flag.NewFlagSet("deploys", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: velori deploys <project-name>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findProject(fs.Arg(0), resolveOrg(*org))
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
		gitInfo := ""
		if d.GitCommitHash != nil && *d.GitCommitHash != "" {
			short := *d.GitCommitHash
			if len(short) > 8 {
				short = short[:8]
			}
			gitInfo = "  " + short
			if d.GitBranch != nil && *d.GitBranch != "" {
				gitInfo += " (" + *d.GitBranch + ")"
			}
			if d.GitDirty != nil && *d.GitDirty {
				gitInfo += "*"
			}
		}
		fmt.Printf("%s%-20s %4d files  %s%s%s\n", current, d.ID, d.FileCount, d.CreatedAt, label, gitInfo)
	}
}

func cmdRollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: velori rollback <project-name> <deploy-id>")
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findProject(fs.Arg(0), resolveOrg(*org))
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

func resolveOrg(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("VELORI_ORG"); env != "" {
		return env
	}
	return loadConfig().Org
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

func cmdVisibility(args []string) {
	fs := flag.NewFlagSet("visibility", flag.ExitOnError)
	org := fs.String("org", "", "Organization slug (defaults to personal org)")
	token := fs.String("token", "", "API token (overrides VELORI_TOKEN)")
	url := fs.String("url", "", "API base URL (overrides VELORI_URL)")
	fs.Parse(args)

	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: velori visibility <project-name> <public|private>")
		os.Exit(1)
	}

	projectName := fs.Arg(0)
	visibility := fs.Arg(1)

	var isPublic bool
	switch visibility {
	case "public":
		isPublic = true
	case "private":
		isPublic = false
	default:
		fmt.Fprintf(os.Stderr, "invalid visibility %q: must be \"public\" or \"private\"\n", visibility)
		os.Exit(1)
	}

	client := newClient(requireToken(*token), resolveURL(*url))
	proj, err := client.findProject(projectName, resolveOrg(*org))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := client.updateVisibility(proj.ID, isPublic); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s is now %s\n", proj.Name, visibility)
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
		org := resolveOrg("")
		fmt.Printf("Token:   %s\n", maskToken(cfg.Token))
		fmt.Printf("URL:     %s\n", url)
		if org != "" {
			fmt.Printf("Org:     %s\n", org)
		}
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
		case "org":
			cfg.Org = args[2]
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
