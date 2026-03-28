import { Copy, Terminal, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from 'sonner'

function CodeBlock({ children }: { children: string }) {
  return (
    <div className="group/code relative rounded-lg border bg-muted/50">
      <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed">
        <code>{children}</code>
      </pre>
      <Button
        variant="ghost"
        size="icon-xs"
        className="absolute right-2 top-2 opacity-0 transition-opacity group-hover/code:opacity-100"
        onClick={() => {
          void navigator.clipboard.writeText(children)
          toast('Copied to clipboard')
        }}
      >
        <Copy />
      </Button>
    </div>
  )
}

function FlagRow({ flag, description }: { flag: string; description: string }) {
  return (
    <div className="flex items-baseline gap-3 py-1.5 first:pt-0 last:pb-0">
      <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{flag}</code>
      <span className="text-sm text-muted-foreground">{description}</span>
    </div>
  )
}

function CommandTab({
  usage,
  description,
  flags,
  example,
  output,
}: {
  usage: string
  description: string
  flags: { flag: string; description: string }[]
  example?: string
  output?: string
}) {
  return (
    <div className="flex flex-col gap-4">
      <CodeBlock>{usage}</CodeBlock>
      <p className="text-sm text-muted-foreground leading-relaxed">{description}</p>
      {flags.length > 0 ? (
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">Flags</p>
          <div className="flex flex-col rounded-lg border p-3">
            {flags.map(({ flag, description: desc }) => (
              <FlagRow key={flag} flag={flag} description={desc} />
            ))}
          </div>
        </div>
      ) : null}
      {example ? (
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">Example</p>
          <CodeBlock>{example}</CodeBlock>
        </div>
      ) : null}
      {output ? (
        <div>
          <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">Output</p>
          <CodeBlock>{output}</CodeBlock>
        </div>
      ) : null}
    </div>
  )
}

const commonFlags = [
  { flag: '--token', description: 'API token (overrides VELORI_TOKEN)' },
  { flag: '--url', description: 'API base URL (overrides VELORI_URL)' },
]

const deployFlags = [
  { flag: '--name', description: 'Project name (overrides directory/zip name)' },
  { flag: '--label', description: 'Deploy label (e.g. "v2 with new header")' },
  { flag: '--private', description: 'Make the project private after deploy' },
  { flag: '--json', description: 'Output full JSON response instead of just the URL' },
  ...commonFlags,
]

const quickStartSteps = [
  { title: 'Install the CLI', code: 'go install github.com/samsolomon/velori/cli@latest' },
  { title: 'Log in', code: 'velori login' },
  { title: 'Deploy', code: 'velori deploy my-site' },
]

export function CLIDocs() {
  return (
    <div className="flex flex-col gap-8">
      <section>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">CLI Documentation</h2>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Terminal className="size-4" />
              Quick Start
            </CardTitle>
            <CardDescription>
              Deploy from your terminal in three steps.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-3">
              {quickStartSteps.map((step, i) => (
                <div key={step.title} className="flex items-start gap-3">
                  <Badge variant="secondary" className="mt-0.5 shrink-0 tabular-nums">{i + 1}</Badge>
                  <div className="flex min-w-0 flex-1 flex-col gap-1.5">
                    <p className="text-sm font-medium">{step.title}</p>
                    <CodeBlock>{step.code}</CodeBlock>
                  </div>
                </div>
              ))}
            </div>
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <ChevronRight className="size-3" />
              Create API tokens in the <span className="font-medium text-foreground">API Tokens</span> section of the dashboard.
            </p>
          </CardContent>
        </Card>
      </section>

      <section>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Command Reference</h2>
        <Card>
          <CardContent className="pt-4">
            <Tabs defaultValue="deploy">
              <TabsList className="w-full">
                <TabsTrigger value="deploy">deploy</TabsTrigger>
                <TabsTrigger value="list">list</TabsTrigger>
                <TabsTrigger value="deploys">deploys</TabsTrigger>
                <TabsTrigger value="rollback">rollback</TabsTrigger>
                <TabsTrigger value="visibility">visibility</TabsTrigger>
                <TabsTrigger value="token">token</TabsTrigger>
                <TabsTrigger value="login">login</TabsTrigger>
                <TabsTrigger value="config">config</TabsTrigger>
              </TabsList>

              <TabsContent value="deploy" className="pt-4">
                <CommandTab
                  usage="velori deploy <path> [flags]"
                  description="Deploy a folder or zip file. Directories are zipped automatically before upload. The project name defaults to the directory or zip filename (.zip extension is stripped)."
                  flags={deployFlags}
                  example='velori deploy landing-page --name my-site --label "v3 redesign"'
                  output="https://velori.dev/~sam/my-site"
                />
              </TabsContent>

              <TabsContent value="list" className="pt-4">
                <CommandTab
                  usage="velori list [flags]"
                  description="List all projects with their deploy counts, visibility, and live URLs."
                  flags={commonFlags}
                  output={`my-site                    3 deploys  public   https://velori.dev/~sam/my-site\nlanding-page               1 deploys  private  https://velori.dev/~sam/landing-page`}
                />
              </TabsContent>

              <TabsContent value="deploys" className="pt-4">
                <CommandTab
                  usage="velori deploys <project-name> [flags]"
                  description="Show deployment history for a project. The current (live) deploy is marked with an asterisk. Project name lookup is case-insensitive."
                  flags={commonFlags}
                  example="velori deploys my-site"
                  output={`* d3f1a2b4c5e6           12 files  2025-03-25T10:30:00Z  v3 redesign\n  a1b2c3d4e5f6            8 files  2025-03-20T14:15:00Z  initial launch`}
                />
              </TabsContent>

              <TabsContent value="rollback" className="pt-4">
                <CommandTab
                  usage="velori rollback <project-name> <deploy-id> [flags]"
                  description="Roll back a project to a previous deployment. Get the deploy ID from velori deploys."
                  flags={commonFlags}
                  example="velori rollback my-site a1b2c3d4e5f6"
                  output="Rolled back my-site to a1b2c3d4e5f6"
                />
              </TabsContent>

              <TabsContent value="visibility" className="pt-4">
                <CommandTab
                  usage="velori visibility <project-name> <public|private> [flags]"
                  description="Set a project's visibility. Public projects are accessible to anyone. Private projects require authentication."
                  flags={commonFlags}
                  example="velori visibility my-site private"
                  output="my-site is now private"
                />
              </TabsContent>

              <TabsContent value="token" className="pt-4">
                <CommandTab
                  usage="velori token [flags]"
                  description="Display info about the current API token — masked token value, authenticated user, and API endpoint."
                  flags={commonFlags}
                  output={`Token:    vtk_test...2345\nUser:     Sam Solomon (sam@velori.dev)\nAPI:      https://velori.dev`}
                />
              </TabsContent>

              <TabsContent value="login" className="pt-4">
                <CommandTab
                  usage="velori login [--token TOKEN] [--url URL]"
                  description="Authenticate and save your API token. Opens your browser to generate a token, then prompts you to paste it. Use --token to skip the browser and provide a token directly (useful for CI/CD)."
                  flags={[
                    { flag: '--token', description: 'API token (skip browser flow)' },
                    { flag: '--url', description: 'API base URL (overrides VELORI_URL)' },
                  ]}
                  example="velori login"
                  output={`Press Enter to open your browser and log in.\nIf the browser didn't open, visit: https://app.velori.dev?cli-auth\n\nPaste your API token: vtk_...\n\nAuthenticated as Sam Solomon (sam@velori.dev)\nToken saved to ~/.velori/config.json\n\nYou're ready to deploy:\n  velori deploy ./my-site`}
                />
              </TabsContent>

              <TabsContent value="config" className="pt-4">
                <CommandTab
                  usage="velori config <show | set key value>"
                  description="View or update CLI configuration. Settings are saved to ~/.velori/config.json."
                  flags={[]}
                  example="velori config set url https://velori.dev"
                  output="Saved url to ~/.velori/config.json"
                />
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
      </section>

      <section>
        <h2 className="mb-4 text-lg font-semibold tracking-tight">Configuration</h2>
        <Card>
          <CardHeader>
            <CardTitle>Configuration</CardTitle>
            <CardDescription>
              The CLI resolves settings from flags, environment variables, the config file, then defaults — in that order.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">Config File</p>
              <div className="rounded-lg border p-3">
                <p className="text-sm text-muted-foreground">
                  Run <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">velori login</code> to save your token, or edit <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">~/.velori/config.json</code> directly.
                </p>
              </div>
            </div>
            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground/70">Environment Variables</p>
              <div className="flex flex-col gap-3 rounded-lg border p-3">
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <code className="font-mono text-xs font-medium">VELORI_TOKEN</code>
                    <Badge variant="outline" className="text-[10px]">optional</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    API token for authentication. Overrides the config file. Tokens use the <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">vtk_</code> prefix.
                  </p>
                </div>
                <div className="border-t" />
                <div className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <code className="font-mono text-xs font-medium">VELORI_URL</code>
                    <Badge variant="outline" className="text-[10px]">optional</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    API base URL. Defaults to <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">http://localhost:8080</code>.
                  </p>
                </div>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Precedence: <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">--flag</code>
              {' > '}
              <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">ENV_VAR</code>
              {' > '}
              <code className="rounded bg-muted px-1 py-0.5 font-mono text-[11px]">config file</code>
              {' > '}
              <span>default value</span>
            </p>
          </CardContent>
        </Card>
      </section>
    </div>
  )
}
