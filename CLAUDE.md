# Contexto — lsm

Documento para qualquer agente/dev que peque no projeto a meio.
Cobre **porquê**, **o quê**, **como está estruturado**, **convenções**,
**o que está fora de scope** e **como adicionar coisas**.
Lê isto **antes** de mexer em código. Continua a ler o código antes de
aplicar mudanças (este doc pode estar desatualizado).

---

## 1. Objetivo

CLI Go (`lsm`) que provisiona servidores **Debian** (12+). Substitui scripts
bash dispersos por:

- Subcomandos idempotentes por módulo (firewall, ssh, docker, ...).
- Menu interativo quando corrido sem args (`sudo lsm`).
- Wizard automático no primeiro arranque (sem config).
- Estado em ficheiro YAML, sincronizado com UFW.
- Distribuído como `.deb` via GitHub Actions.

**Foco:** servidor Debian para correr containers (Docker rootless) + acesso
controlado por whitelist de IPs.

---

## 2. História / origem

1. Começou em `/Users/pedro/Desktop/Debian/setup-server.sh` — bash monolítico
   (UFW + SSH + Docker + MySQL).
2. Refactorizado para `/Users/pedro/Desktop/Debian/` — modular bash com
   `setup.sh` orquestrador + `modules/01-firewall.sh`, `02-ssh.sh`, etc.
3. MySQL removido (app-specific, não base server).
4. Convertido para Go em `/Users/pedro/Desktop/lsm/` — projeto atual.
5. Vai ser publicado em GitHub repo próprio (owner: pedro / erp24).

---

## 3. Decisões arquiteturais (com porquê)

| Decisão | Porquê |
|---|---|
| Go em vez de bash | Idempotência mais limpa, testável, binário único, wrappers tipados sobre system tools. Bash continua a ser quem faz o trabalho real (apt, ufw, systemctl) — Go orquestra. |
| Cobra para CLI | Standard de facto, dá help/version/completion grátis. |
| YAML direto (`gopkg.in/yaml.v3`), **sem viper** | Viper era overkill (precisávamos só de Load/Save). Menos uma dep. |
| Sem TUI lib (huh/bubbletea) | User pediu "só CLI". Prompts via `bufio` stdlib chegam. |
| `internal/runner` wrappa `os/exec` | Centraliza logging, dry-run, root check. |
| Binário em `/usr/sbin/lsm` | Convenção Debian para tools que precisam de root. Em PATH do sudo. |
| Config em `/etc/lsm/config.yaml` | Convenção `/etc/<projeto>/`. Path default no flag `--config`. |
| State separado (`state.yaml`) | Mantém `config.yaml` editável-à-mão; state é gerado. |
| Whitelist única `network.allowed_ips` (não por-porta) | Simplicidade. Vazia = portas abrem a todos. |
| Política `auto_open_ports: true\|false\|ask` | User pode controlar quando módulos abrem firewall. `ask` default. |
| Wizard só no primeiro run; menu nas restantes | Match com o que user pediu: "Setup Inicial (se for), senão Validar setup". |
| Modules devem registar portas em `state.yaml` quando abrem em UFW | Permite `add-ip`/`remove-ip` sincronizar todas as portas geridas sem hardcode. |

---

## 4. Fora de scope (decisão explícita do user)

- **MySQL / Postgres / Redis / compose stacks** — app-specific, fora do `lsm`.
- **SSH keys** (deploy `authorized_keys` + desligar `PasswordAuth`) — user dispensou nesta fase.
- **Swap** — user dispensou.
- **WireGuard** — adiar; user vai pedir como módulo no futuro.
- **Reverse proxy / TLS / certbot / backups** — não pedido.
- **RHEL / Alpine / Arch / não-systemd** — só Debian, só systemd.

Quando user pedir wireguard ou outro: segue o **padrão de módulo** (secção 8).

---

## 5. File layout

```
lsm/
├── main.go                    # entrypoint; version injetado via -ldflags
├── go.mod / go.sum
├── README.md                  # docs públicas (PT)
├── CLAUDE.md                  # este ficheiro
├── config.example.yaml        # template, vai para /etc/lsm/ no .deb
├── .github/workflows/
│   └── release.yml            # workflow_dispatch → tag + .deb release
├── cmd/                       # cobra subcommands
│   ├── root.go                # rootCmd + flags globais + shouldOpen()
│   ├── menu.go                # menu interativo + runAllModules + helpers
│   ├── init.go                # wizard
│   ├── validate.go            # checks (cresce a cada módulo novo)
│   ├── all.go                 # `lsm all` → runAllModules()
│   ├── ip.go                  # add-ip / remove-ip + sync UFW
│   ├── firewall.go            # UFW bootstrap (idempotente)
│   ├── ssh.go                 # user + harden + open port + state register
│   ├── docker.go              # install + rootless
│   ├── fail2ban.go            # jail.local
│   ├── upgrades.go            # unattended-upgrades
│   ├── timesync.go            # parent + sub-cmds: sync, status
│   ├── sysctl.go              # /etc/sysctl.d/99-lsm.conf
│   └── hostname.go            # hostnamectl + /etc/hosts
└── internal/
    ├── config/config.go       # Config struct + Load/Save + IP add/remove
    ├── state/state.go         # State struct + ManagedPort + Add/Remove
    ├── prompt/prompt.go       # Ask, AskInt, AskIPOrCIDR, Confirm, Choose
    ├── runner/runner.go       # Run, Capture, Stdin, DryRun, RequireRoot
    ├── ufw/ufw.go             # Install, Allow*, Delete*, OpenWhitelisted
    ├── ssh/ssh.go             # CreateUser, Harden, OpenFirewall
    ├── docker/docker.go       # RemoveConflicts, InstallRepo, InstallEngine, SetupRootlessUser
    ├── fail2ban/fail2ban.go   # Install, WriteJailConfig, Enable
    ├── upgrades/upgrades.go   # Install, EnablePeriodic
    ├── timesync/timesync.go   # SetTimezone, Enable, ForceSync, Synced, NTPEnabled
    ├── sysctl/sysctl.go       # Body (const), Write, Apply, Get, Expected
    └── hostname/hostname.go   # Set, UpdateHosts, Apply, Current
```

---

## 6. Schema do `config.yaml`

`config.yaml` representa **intenção** — sem segredos, sem dados derivados.
Whitelist de IPs **NÃO** está aqui (lê-se direto do UFW). Password SSH **NÃO**
está aqui (pedida em runtime quando o user é criado).

```yaml
timezone: Europe/Lisbon          # default se omitido
hostname: srv01                  # opcional; vazio → módulo hostname não corre
fqdn: srv01.exemplo.com          # opcional

ssh:
  port: 2210                     # required
  user: dev24                    # required (password pedida em runtime)

docker:
  rootless_user: docker24        # required

network:
  auto_open_ports: ask           # true | false | ask
```

Campos com default em `internal/config/config.go::validate()`. Required ones
falham com erro explícito.

---

## 7. Schema do `state.yaml`

Gerido pelo lsm. NÃO editar à mão.

```yaml
managed_ports:
  - {port: 2210, proto: tcp, label: SSH}
```

Cada módulo que abre porta whitelisted **deve** registar via
`st.AddPort(state.ManagedPort{...})` + `st.Save()`.

---

## 8. Como adicionar módulo novo X

Padrão obrigatório (segue ssh.go ou fail2ban.go como template):

1. **`internal/X/X.go`** — funções puras:
   - `Installed() bool`
   - `Install() error`
   - `Configure(...) error` (toma valores do config, não loaders)
   - `Enable() error`
   - `Status()` ou similar (read-only, p/ validate)
   - **Idempotente**. Usa `runner.Run/Capture/Stdin`. Devolve `error`.

2. **`cmd/X.go`** — cobra subcommand:
   ```go
   var XCmd = &cobra.Command{
       Use:   "X",
       Short: "...",
       RunE: func(cmd *cobra.Command, args []string) error {
           if err := runner.RequireRoot(); err != nil { return err }
           cfg, err := config.Load(cfgFile)
           if err != nil { return err }
           runner.Section("X: ...")
           // chama internal/X/* em ordem
           return nil
       },
   }
   func init() { rootCmd.AddCommand(XCmd) }
   ```

3. **`cmd/menu.go`** — adiciona ao `chooseAndRunModule()` (interativo) e
   `runAllModules()` (com `skip:` se depender de config opcional).

4. **`cmd/validate.go`** — adiciona checks (`check("nome", bool, "detalhe")`).

5. Se introduzir campo novo no config:
   - `internal/config/config.go::Config` struct (yaml tag).
   - Default em `validate()`.
   - Prompt em `cmd/init.go::runWizard`.
   - `config.example.yaml`.

6. Atualiza `README.md` (secção Comandos + Como funcionam os módulos).

7. Atualiza este `CLAUDE.md` (secção 9 — lista de módulos).

8. **Não** mudes a ordem em `runAllModules` sem perceber dependências
   (firewall sempre primeiro; ssh depois de firewall; docker antes de fail2ban).

---

## 9. Módulos atuais (ordem em `lsm all`)

| # | Cmd | O que faz | Lê do config |
|---|---|---|---|
| 1 | `firewall` | Install UFW, defaults deny/allow, abre 22 (bootstrap), enable | — |
| 2 | `timesync` | `set-timezone` + `set-ntp true` + enable timesyncd | `timezone` |
| 3 | `sysctl` | Escreve `/etc/sysctl.d/99-lsm.conf` + `sysctl --system` | — (hardcoded) |
| 4 | `hostname` | `hostnamectl` + atualiza `/etc/hosts` | `hostname`, `fqdn` (skip se vazio) |
| 5 | `ssh` | Cria user **fora do grupo sudo** (operator-only), grava `/etc/sudoers.d/lsm` com NOPASSWD restrito (`validate` / `update-server` / `timesync status`), hardening sshd, abre porta UFW (1ª vez = a todos), regista state | `ssh.user`, `ssh.port`, `network.auto_open_ports` |
| 6 | `docker` | Remove conflitos, install Docker CE, rootless setup user dedicado | `docker.rootless_user` |
| 7 | `fail2ban` | Install, escreve `jail.local` (port=ssh.port, ignoreip = `ufw.SpecificSources` da porta SSH) | `ssh.port` |
| 8 | `upgrades` | Install + escreve `20auto-upgrades` + enable service | — |

Cada módulo regista-se em `state.yaml::installed_modules` após sucesso.
`validate` só faz checks contra módulos com flag — outros aparecem como `[--]`.

Comandos não-modulares:
- `lsm update-server` — `apt update + upgrade + autoremove + autoclean`,
  com `needrestart` para auto-restart de serviços. Detecta reboot pendente
  e oferece: agora / amanhã 04:00 (`systemd-run --on-calendar`) / adiar.

Sub-cmds especiais:
- `lsm timesync sync` — força re-sync (restart timesyncd)
- `lsm timesync status` — `timedatectl status`

---

## 10. Subcomandos top-level (todos)

```
lsm                       # menu interativo
lsm init                  # wizard
lsm validate              # audita estado vs config (read-only)
lsm all                   # corre todos os módulos por ordem
lsm update-server         # apt update + upgrade + autoremove + auto-restart
lsm add-ip [IP]           # atalho: ALLOW from IP em TODAS as portas geridas
lsm remove-ip [IP]        # atalho: DELETE allow from IP (com fallback p/ Anywhere)
lsm port add <P>/<PROTO> [LABEL]    # registar + abrir (--restrict abre fechado)
lsm port remove <P>/<PROTO>          # fechar + remover de state
lsm port allow <P>/<PROTO> <IP>      # ALLOW from IP só nessa porta
lsm port revoke <P>/<PROTO> <IP>     # DELETE allow from IP só nessa porta
lsm port list                        # tabela: portas geridas + sources UFW
lsm firewall|ssh|docker|fail2ban|upgrades|timesync|sysctl|hostname
lsm timesync sync|status
```

Flags globais:
- `--config <path>` (default `/etc/lsm/config.yaml`)
- `--dry-run` (loga, não executa)
- `-y/--yes` (auto-confirma prompts)
- `--version`

---

## 10b. Modelo de permissões (admin vs operator)

Dois papéis. Decididos em runtime, não persistidos.

- **admin** = real root login. Detetado por `uid==0 && $SUDO_USER == ""`.
  Caminho típico: cloud console / IPMI / `su -` com password do root.
  Pode tudo (init, ssh, docker, firewall, add-ip, remove-ip, all, ...).
- **operator** = `sudo lsm` invocado de user não-root (ex: dev24).
  `$SUDO_USER` está definido. Só pode: `validate`, `update-server`,
  `timesync status`, `ver overview`. Tudo o resto é rejeitado por
  `RequireAdmin()` em `cmd/root.go`.

`RequireAdmin()` está aplicado em: firewall, ssh, docker, fail2ban,
upgrades, sysctl, hostname, init, add-ip, remove-ip. Operator-class
cmds usam só `runner.RequireRoot()`.

dev24 (ou seja qual for `ssh.user`) **NÃO** está no grupo sudo. Único
caminho de privilégio é via lsm sudoers drop-in scoped aos subcmds
operator. Para destructive: precisa-se de root login de outra via.

## 11. Semântica da whitelist de IPs

**Source of truth = UFW**. Config NÃO armazena IPs. Operações lêem `ufw status`
e modificam regras direto.

Estado da porta gerida pode ser:
- **Aberta a todos** → `ALLOW Anywhere` no UFW (sem regras `from`).
- **Restrita** → uma ou mais regras `ALLOW from <ip> to any port P proto T`,
  e SEM regra `Anywhere` em paralelo.

`lsm add-ip X`:
1. Para cada porta em `state.yaml::managed_ports`: `ufw allow from X to any port P proto T`.
2. Se a porta tinha regra `Anywhere`, remove-a — passa a estar restrita.

`lsm remove-ip X`:
1. Para cada porta gerida: `ufw delete allow from X ...`.
2. Se a porta ficou sem IPs específicos AND não estava `Anywhere`:
   `ufw allow P/T <label>` para evitar locked-out (reabre a todos).

`fail2ban` lê `ignoreip` direto de `ufw.SpecificSources(sshPort, "tcp")` — IPs
que já têm acesso explícito ao SSH não são banidos.

Visualização (`Ver IPs / portas geridas` no menu) usa `ufw.AllowedSources` por
porta gerida + união disso como "whitelist efetiva".

---

## 12. Convenções de código

- **Erros**: `fmt.Errorf("contexto: %w", err)` com wrap.
- **Logs**: PT amigável ("UFW já presente.", "User 'X' criado.").
- **Sections**: `runner.Section("X: o que vou fazer")` antes de cada bloco.
- **Status**: `runner.Log(...)` para linhas de progresso.
- **Escrita de ficheiros system**: `os.WriteFile(path, []byte(body), 0644)` — preferir sobre `tee` via stdin (mais limpo, sem output duplicado).
- **Comandos shell**: `runner.Run("apt-get", "install", "-y", "-qq", "pkg")` — args separados, sem string interpolation.
- **Comandos que podem falhar**: `runner.TryRun(...)` ignora erro (ex: remover pkg que pode não existir).
- **Captura de output**: `runner.Capture(...)` devolve `(string, error)`. Erro discardable se semântica é "vazio = não existe".
- **Stdin**: `runner.Stdin(body, "cmd", "arg")` para piping.
- **Cobra**: cada subcommand em ficheiro próprio (`cmd/X.go`), regista via `init()`.
- **`SilenceUsage: true`** já no rootCmd — erros não imprimem usage spam.
- **`validate`** deve devolver erro se algum check FAIL → exit code != 0 → CI-friendly.
- **Commits/PRs**: NÃO incluir `Co-Authored-By: Claude ...` nem `🤖 Generated with [Claude Code]` em commits ou bodies de PRs. Autoria é só do user.

---

## 13. Build & release

### Local

```bash
go build -o lsm .
go build -ldflags "-X main.version=0.1.0" -o lsm .   # com versão
GOOS=linux GOARCH=amd64 go build -o lsm-linux .      # cross-compile
```

### CI (`.github/workflows/release.yml`)

- Trigger: `workflow_dispatch` com inputs `version` (sem `v`) + `branch`.
- Matrix: amd64 + arm64.
- Build estático Go (`CGO_ENABLED=0`).
- Empacota com `dpkg-deb --build`:
  - Binário → `/usr/sbin/lsm`
  - Config exemplo → `/etc/lsm/config.example.yaml`
  - README → `/usr/share/doc/lsm/README.md`
- Dois assets por arch: `lsm_<version>_<arch>.deb` (immutable) + `lsm_<arch>.deb` (alias para `releases/latest/download/`).
- Release notes auto + body custom com one-liner de install.
- Tag `vX.Y.Z` criada automaticamente.

### Postinst behavior

Distingue install fresco (`$2` vazio) vs upgrade (`$2 = OLD_VERSION`):

- **Fresh**: imprime paths + `sudo lsm` next step.
- **Upgrade**: imprime "X → Y" + sugere `diff config vs example` + `sudo lsm validate`.

`config.yaml` + `state.yaml` **NÃO** estão no manifest do package → preservados sempre.
`config.example.yaml` é atualizado a cada install.

---

## 14. Locations no servidor

| Path | Conteúdo | Gerido por |
|---|---|---|
| `/usr/sbin/lsm` | binário | dpkg |
| `/etc/lsm/config.yaml` | source of truth | user / wizard |
| `/etc/lsm/config.example.yaml` | template do schema | dpkg (atualizado em upgrades) |
| `/etc/lsm/state.yaml` | portas geridas | lsm (read/write) |
| `/etc/sysctl.d/99-lsm.conf` | sysctl tuning | lsm sysctl |
| `/etc/fail2ban/jail.local` | jail config | lsm fail2ban |
| `/etc/apt/apt.conf.d/20auto-upgrades` | unattended-upgrades periodic | lsm upgrades |
| `/etc/sudoers.d/lsm` | NOPASSWD para `/usr/sbin/lsm` ao SSH user | lsm ssh |
| `/usr/share/doc/lsm/README.md` | docs | dpkg |

---

## 15. User profile

- **Email**: pedro.santos@erp24.pt
- **Idioma**: PT (mensagens, prompts, README, este doc).
- **Plataforma dev**: macOS (darwin/arm64). Server target: Debian linux.
- **Conta GitHub**: vai criar repo próprio para isto.
- **Estilo de comunicação**: terse (caveman mode ativo na conversa). Resposta seca e direta. Sem fluff/preâmbulos.
- **Fluxo**: faz perguntas curtas, prefere ver opções com trade-offs antes de decidir, depois manda implementar. Aprecia validate/dry-run.

---

## 16. Coisas a saber antes de mudar código

- **Não introduzir deps novas sem pensar duas vezes**. Atualmente: `cobra`, `yaml.v3` + indireto.
- **Manter mensagens em PT**.
- **Idempotência sempre**. Cada módulo verifica estado antes de agir. Re-correr 10× tem o mesmo efeito que correr 1×.
- **Validate cresce com módulos**. Se adicionas módulo, adiciona checks ao `cmd/validate.go`.
- **Wizard cresce com config**. Campo novo → prompt no wizard.
- **`config.example.yaml`** tem de espelhar o schema atual (é o template que vai para `/etc/lsm/`).
- **CLI deve continuar a funcionar sem args** → menu. `lsm all -y` deve funcionar sem prompts.
- **Não partir o postinst/control do .deb** sem testar (`dpkg-deb --build` + `--info` + `--contents`).
- **Versão é injetada via ldflags** — `main.version` é `"dev"` se não for set.

---

## 17. Próximos módulos planeados (por user)

- **wireguard** — VPN. Config provável: peer config inline ou path para conf. Server ou client peer.

(Lista atualiza-se à medida que user pede.)

---

## 18. Coisas que provavelmente vão precisar de cuidado

- **Password SSH não persiste**. É pedida só quando o user ainda não existe. Se rodar `lsm ssh` sobre user existente, password mantém-se a que já estava. Para reset: `passwd <user>` manual.
- **Sysctl `net.ipv6.conf.all.forwarding=1`** pode ser indesejado em hosts puramente IPv4. Considerar tornar opcional via config.
- **Fail2ban backend `systemd`** assume rsyslog/journald. Em Debian default funciona. Em hosts customizados pode falhar.
- **Docker rootless**: `dockerd-rootless-setuptool.sh install` precisa de `machinectl shell` que precisa de `systemd-container` (geralmente lá). Validar em distros minimalistas.
- **`lsm timesync` em containers/LXC** sem CAP_SYS_TIME falha. Não tratamos esse caso.
- **`hostname` em containers** pode estar bloqueado pelo runtime. `hostnamectl` retorna erro silencioso? Verificar.
