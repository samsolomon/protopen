package main

import (
	"flag"
	"fmt"
	"os"
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
  deploy <path>   Deploy a folder or zip to Velori
  list            List your projects
  deploys <name>  Show deploy history for a project
  rollback <name> <deploy-id>  Roll back to a previous deploy
  token           Show current token info

Environment:
  VELORI_TOKEN    API token for authentication
  VELORI_URL      API base URL (default: http://localhost:8080)`)
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
	client := newClient(resolveToken(*token), resolveURL(*url))

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

	client := newClient(resolveToken(*token), resolveURL(*url))
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

	client := newClient(resolveToken(*token), resolveURL(*url))
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

	client := newClient(resolveToken(*token), resolveURL(*url))
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

	t := resolveToken(*token)
	if t == "" {
		fmt.Fprintln(os.Stderr, "no token set (VELORI_TOKEN or --token)")
		os.Exit(1)
	}

	client := newClient(t, resolveURL(*url))
	user, err := client.getSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	masked := t[:8] + "..." + t[len(t)-4:]
	fmt.Printf("Token:    %s\n", masked)
	fmt.Printf("User:     %s (%s)\n", user.Name, user.Email)
	fmt.Printf("API:      %s\n", resolveURL(*url))
}

func resolveToken(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("VELORI_TOKEN")
}

func resolveURL(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("VELORI_URL"); env != "" {
		return env
	}
	return "http://localhost:8080"
}

func stripZipExt(name string) string {
	if len(name) > 4 && name[len(name)-4:] == ".zip" {
		return name[:len(name)-4]
	}
	return name
}
