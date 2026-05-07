# lsm — Linux Server Manager

Provisionamento idempotente de servidores **Debian** num único binário Go.
Substitui dezenas de scripts bash dispersos por uma CLI com menu interativo
e subcomandos para CI/automação.

> Só Debian (testado em 12+). Requer `systemd`.

---

## Porquê

Configurar um servidor novo envolve sempre os mesmos passos: firewall,
SSH hardening, Docker, fail2ban, security updates, timezone, sysctl, hostname.
Tipicamente há um script bash gigante por equipa, frágil e não-reversível.

`lsm` resolve isso com:

- **Idempotência**: cada módulo verifica estado antes de agir, podes correr 10×.
- **Modular**: corres só o que precisas (`lsm fail2ban`) ou tudo (`lsm all`).
- **Validation**: `lsm validate` audita o sistema vs config sem mudar nada.
- **State-aware**: portas abertas pelo lsm ficam tracked; adicionar/remover
  IPs da whitelist sincroniza UFW automaticamente.
- **Único binário Go**: zero deps Python/Ruby; cross-compile fácil.
- **Interativo por defeito**: corres `sudo lsm`, segues o menu.

---

## Instalar

> Repo: **https://github.com/santospedro1993/LittleServerManager**
> Releases: **https://github.com/santospedro1993/LittleServerManager/releases**

### Via .deb (recomendado) — sempre a versão mais recente

Detecta arch automaticamente (`amd64` ou `arm64`):

```bash
ARCH=$(dpkg --print-architecture)
rm -f lsm_*.deb                                   # limpa downloads antigos
wget -O lsm_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/lsm_${ARCH}.deb
sudo apt install -y ./lsm_${ARCH}.deb
sudo lsm
```

> ⚠️ `wget -O` força sobrescrever o ficheiro. Sem isso, `wget` cria
> `lsm_amd64.deb.1` em re-downloads e o `apt install` instala o
> **antigo** que continua em disco. Por isso o `rm -f` antes.

> Usar `apt install ./file.deb` em vez de `dpkg -i` resolve dependências
> automaticamente.

### Versão específica

```bash
VER=0.1.0
ARCH=$(dpkg --print-architecture)
rm -f lsm_*.deb
wget -O lsm_${VER}_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/download/v${VER}/lsm_${VER}_${ARCH}.deb
sudo apt install -y ./lsm_${VER}_${ARCH}.deb
```

A instalação cria `/etc/lsm/` e imprime os caminhos relevantes + próximo passo.

### Build a partir do código

```bash
git clone https://github.com/USER/lsm.git
cd lsm
go build -o lsm .
sudo install -m 755 lsm /usr/sbin/lsm
sudo mkdir -p /etc/lsm
sudo install -m 644 config.example.yaml /etc/lsm/config.example.yaml
```

Requer Go 1.23+.

### Build .deb localmente

```bash
GOOS=linux GOARCH=amd64 go build -o lsm .
# repete o bloco "Build .deb" do .github/workflows/release.yml
```

---

## Quick start

```bash
sudo lsm
```

**Primeiro arranque** (sem `/etc/lsm/config.yaml`):
1. **Apt update + upgrade** automático antes de tocar em config (kernel/security
   patches primeiro). Se for preciso reboot, lsm pergunta e sai — re-corres
   depois e retoma daqui.
2. **Wizard** pergunta valores: timezone (default `Etc/UTC`), hostname, SSH
   user/port, docker rootless user, política `auto_open_ports`. Modules
   baseline (firewall, ssh, sysctl, timesync, hostname, fail2ban, upgrades)
   são sempre instalados; **só docker é opt-in**.
3. **Passwords** dos users SSH e docker rootless são pedidas em runtime
   (`stty -echo` + double-prompt). Não ficam em ficheiro.
4. Corre módulos selecionados.
5. Pergunta se quer **auto-launch** do menu no login do SSH user (escreve
   bloco em `~/.profile`).

**Run subsequente** (config existe):
- Menu top-level: **Status / Validate / System / Network / Modules / Setup wizard / x**.
- Submenus navegam com número, `x` recua/sai.
- dev24 (operator) só vê: Status / Validate (read-only) / System (update + reboot) / Network (list).
- root (admin) vê tudo.

---

## Upgrade

Mesma instalação substitui o binário e atualiza o template de config.
**Nada do teu setup é perdido.**

```bash
ARCH=$(dpkg --print-architecture)
rm -f lsm_*.deb                                   # crítico: senão apt instala o antigo
wget -O lsm_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/lsm_${ARCH}.deb
sudo apt install -y ./lsm_${ARCH}.deb
```

O que acontece em cada upgrade:

| Ficheiro | Ação |
|---|---|
| `/usr/sbin/lsm` | **Substituído** pela nova versão |
| `/etc/lsm/config.example.yaml` | **Atualizado** (reflete schema novo) |
| `/etc/lsm/config.yaml` | **Preservado** (a tua config fica intacta) |
| `/etc/lsm/state.yaml` | **Preservado** (portas geridas tracked) |
| `/usr/share/doc/lsm/README.md` | Atualizado |

O `postinst` deteta upgrade e mostra:
- Versão antiga → nova
- Comando de diff: `diff /etc/lsm/config.yaml /etc/lsm/config.example.yaml`
- Sugere `sudo lsm validate` para confirmar que tudo continua coerente

### Comparar versão atual vs latest publicada

```bash
lsm --version
curl -s https://api.github.com/repos/santospedro1993/LittleServerManager/releases/latest | grep tag_name
```

### Após upgrade — validar

```bash
sudo lsm validate
```

Se o schema mudou e a tua config tem campos novos em falta, **lsm vai usar
defaults sensatos**. Para incorporares manualmente:

```bash
sudo diff /etc/lsm/config.yaml /etc/lsm/config.example.yaml
sudo $EDITOR /etc/lsm/config.yaml
sudo lsm validate
```

### Downgrade

Funciona via `apt install` apontando para `.deb` mais antigo. Mas o `state.yaml`
pode ter formato mais novo — re-correr módulos regenera-o.

### Remover

```bash
sudo apt remove lsm        # tira binário, mantém /etc/lsm
sudo apt purge lsm         # mesma coisa (não há conffiles)
sudo rm -rf /etc/lsm       # remoção total da config + state
```

---

## Localizações

| O quê | Onde | Notas |
|---|---|---|
| Binário real | `/usr/sbin/lsm` | Convenção Debian para tools que precisam de root |
| Wrapper PATH | `/usr/local/bin/lsm` | `exec sudo /usr/sbin/lsm "$@"` — `lsm` plain funciona p/ user não-root |
| Config | `/etc/lsm/config.yaml` | **Edita aqui**. Source of truth |
| Exemplo | `/etc/lsm/config.example.yaml` | Template, copia/edita |
| State | `/etc/lsm/state.yaml` | Gerido pelo lsm; portas geridas + módulos instalados |
| sshd drop-in | `/etc/ssh/sshd_config.d/99-lsm.conf` | Port + PermitRootLogin no + PasswordAuthentication yes |
| Sudoers | `/etc/sudoers.d/lsm` | NOPASSWD blanket em `/usr/sbin/lsm` para SSH user (gate em-app) |
| needrestart | `/etc/needrestart/conf.d/99-lsm.conf` | Modo list-only (sem dialog interativo no apt) |
| Auto-launch | `~/.profile` (SSH user) | Bloco marcado: corre `sudo lsm` no login interativo |
| Docs | `/usr/share/doc/lsm/README.md` | Este ficheiro |

Override do path da config: `sudo lsm --config /caminho/outro.yaml`

---

## Configuração

`/etc/lsm/config.yaml` representa **intenção** — sem segredos, sem dados
derivados (whitelist de IPs vem do estado real do UFW; passwords são pedidas
em runtime).

```yaml
timezone: Etc/UTC
hostname: srv01                  # vazio → módulo hostname não corre
fqdn: srv01.example.com          # opcional

ssh:
  port: 2210                     # alternativa à 22 reduz brute-force
  user: dev24                    # password pedida ao criar user

docker:
  rootless_user: docker24        # password pedida ao criar user

network:
  # Política para módulos que precisam de abrir portas em UFW:
  #   ask   → pergunta caso a caso (recomendado)
  #   true  → abre automaticamente
  #   false → nunca abre (gestão manual)
  auto_open_ports: ask

modules:
  firewall: true   # baseline (sempre)
  ssh: true        # baseline (sempre)
  sysctl: true     # baseline
  timesync: true   # baseline
  hostname: true   # baseline (skip se hostname vazio)
  fail2ban: true   # baseline
  upgrades: true   # baseline
  docker: true     # opt-in via wizard
```

---

## Comandos

### Top-level

| Comando | Descrição |
|---|---|
| `lsm` | Menu interativo |
| `lsm init` | Wizard de config |
| `lsm validate` | Audita sistema vs config (read-only) |
| `lsm all` | Corre módulos selecionados em `config.modules.*` |
| `lsm status [--live]` | CPU / RAM / disk / network. `--live` faz refresh 2s |
| `lsm system update` | apt update + upgrade + autoremove (sem restart inline; pergunta reboot) |
| `lsm system reboot` | Reboot agora / agendar 04:00 / adiar |
| `lsm port add P/PROTO [LABEL]` | Registar + abrir porta (`--restrict` para abrir fechada) |
| `lsm port remove P/PROTO` | Fechar + remover de state |
| `lsm port allow P/PROTO IP` | ALLOW from IP só nessa porta |
| `lsm port revoke P/PROTO IP` | DELETE allow from IP só nessa porta |
| `lsm port list` | Tabela: portas geridas + sources UFW |
| `lsm add-ip [IP]` | Atalho: ALLOW from IP em **todas** portas geridas |
| `lsm remove-ip [IP]` | Atalho: DELETE em todas portas geridas |

### Módulos

| Comando | Descrição |
|---|---|
| `lsm firewall` | UFW: install, defaults deny/allow, abre 22 (bootstrap só na 1ª vez) |
| `lsm ssh` | Cria user (pede password), drop-in `sshd_config.d/99-lsm.conf`, abre porta UFW |
| `lsm docker` | Instala Docker CE, dependências rootless (slirp4netns, fuse-overlayfs, systemd-container), cria user (pede password), `dockerd-rootless-setuptool.sh install` |
| `lsm fail2ban` | jail.local com `port=ssh.port` + ignoreip lido do UFW |
| `lsm upgrades` | unattended-upgrades + periodic config |
| `lsm timesync` | systemd-timesyncd + timezone |
| `lsm timesync sync` | Força re-sync NTP |
| `lsm timesync status` | `timedatectl status` |
| `lsm sysctl` | Escreve `/etc/sysctl.d/99-lsm.conf` |
| `lsm hostname` | `hostnamectl set-hostname` + `/etc/hosts` |

### Modelo de permissões

| Role | Como detectado | Pode |
|---|---|---|
| **admin** | uid 0 + (`$SUDO_USER` vazio ou `root`) | tudo |
| **operator** | uid 0 + `$SUDO_USER` é user não-root (ex: dev24) | só read-only + system update/reboot |

Operator é tipicamente o SSH user (default `dev24`) que **não** está no grupo
`sudo`. Privilégio vem só da entrada NOPASSWD em `/etc/sudoers.d/lsm`; lsm
filtra por subcmd em-app. Operações destrutivas precisam de root login
(console / IPMI / `su -`).

### Flags globais

| Flag | Descrição |
|---|---|
| `--config <path>` | Caminho do config file (default `/etc/lsm/config.yaml`) |
| `--dry-run` | Loga ações sem executar |
| `-y, --yes` | Auto-confirma prompts (CI-friendly) |

---

## Como funcionam os módulos

### firewall
- Instala UFW se faltar.
- `default deny incoming / allow outgoing`.
- Abre porta 22 (bootstrap, evita lockout antes do SSH module mudar a porta).
- Ativa UFW.
- **Idempotente**: re-correr não duplica regras.

### ssh
- Cria user (`ssh.user`) + pede password em runtime se ainda não existir.
- **Não** adiciona ao grupo sudo (modelo operator-only).
- Drop-in `/etc/ssh/sshd_config.d/99-lsm.conf`:
  - `Port` = `ssh.port`
  - `PermitRootLogin no`
  - `PasswordAuthentication yes` (até teres ssh-keys)
- Valida com `sshd -t` antes de `systemctl reload-or-restart ssh`.
- Abre nova porta em UFW respeitando `auto_open_ports`.
- Regista porta em `state.yaml`.
- Grava `/etc/sudoers.d/lsm` com NOPASSWD em `/usr/sbin/lsm` (gate em-app).
- Wrapper `/usr/local/bin/lsm` é criado pelo `PersistentPreRunE` (idempotente).

> ⚠️ **Após confirmar SSH na nova porta**, remove a 22:
> ```bash
> sudo ufw delete allow 22/tcp
> ```

### docker
- Skip `RemoveConflicts` se docker já instalado (evita destruir engine ativo).
- Caso contrário remove `docker.io`, `podman`, `runc` (não toca em docker-ce*).
- Instala Docker CE oficial via repo apt + `slirp4netns` + `fuse-overlayfs` + `systemd-container`.
- Cria user `docker.rootless_user` + pede password em runtime se ainda não existir.
- subuid/subgid + `loginctl enable-linger`.
- `dockerd-rootless-setuptool.sh install` via `machinectl shell <user>@`.
- Desativa Docker rootful (`docker.service` + `docker.socket`).

### fail2ban
- `apt install fail2ban`.
- `/etc/fail2ban/jail.local`:
  - `bantime=1h`, `findtime=10m`, `maxretry=5`
  - `ignoreip = 127.0.0.1/8 ::1 + ufw.SpecificSources(ssh.port, "tcp")`
  - `[sshd]` enabled na `ssh.port`
  - `backend=systemd`
- Restart fail2ban.

### upgrades
- `apt install unattended-upgrades apt-listchanges`.
- Escreve `/etc/apt/apt.conf.d/20auto-upgrades`:
  - `Update-Package-Lists "1"`
  - `Unattended-Upgrade "1"`
  - `AutocleanInterval "7"`
- Enable `unattended-upgrades.service`.
- Origens default = security only (Debian default).

### timesync
- Usa `systemd-timesyncd` (default Debian, sem instalar).
- `timedatectl set-timezone <config.timezone>`.
- `timedatectl set-ntp true` + `systemctl enable --now systemd-timesyncd`.
- `lsm timesync sync` faz `systemctl restart systemd-timesyncd` (force re-sync).

### sysctl
- Escreve `/etc/sysctl.d/99-lsm.conf` com:
  - **Network hardening**: `rp_filter`, `accept_redirects=0`, `tcp_syncookies`, anti-smurf, `log_martians`.
  - **Forwarding**: `net.ipv4.ip_forward=1` (Docker).
  - **TCP perf**: `somaxconn=1024`, keepalive tuning.
  - **VM**: `vm.swappiness=10`.
  - **fs**: `inotify.max_user_watches=524288` (node/webpack), `file-max`.
- Aplica com `sysctl --system`.
- Re-correr sobrescreve **só** o ficheiro `99-lsm.conf` — não toca em outros sysctl drops.

### hostname
- `hostnamectl set-hostname <config.hostname>`.
- Atualiza `/etc/hosts`:
  - Garante linha `127.0.0.1 localhost`.
  - Substitui ou adiciona linha `127.0.1.1 <fqdn> <hostname>` (Debian convention).
- Skip se `config.hostname` vazio.

---

## Portas + whitelist de IPs

**Source of truth = UFW**. Config NÃO armazena IPs nem portas. lsm parsea
`ufw status` para saber estado real, e `state.yaml::managed_ports` é só
a lista de portas que o lsm gere.

Cada porta gerida tem o seu próprio set de IPs permitidos. Estados possíveis:

- **Aberta a todos**: regra `Anywhere` no UFW (sem `from <ip>`).
- **Restrita**: 1+ regras `ALLOW from <ip> to any port P proto T`, e SEM `Anywhere`.

### Por porta (recomendado)

```bash
sudo lsm port add 3306/tcp "MySQL"            # abre a todos + regista
sudo lsm port allow 3306/tcp 1.2.3.4          # restringe só a esse IP
sudo lsm port allow 3306/tcp 192.168.1.0/24   # adiciona outro
sudo lsm port revoke 3306/tcp 1.2.3.4         # tira IP (reabre a todos se ficou vazio)
sudo lsm port remove 3306/tcp                 # fecha + remove de state
sudo lsm port list                            # tabela de estado
```

### Atalho global (aplica a todas as portas geridas)

```bash
sudo lsm add-ip 1.2.3.4
sudo lsm remove-ip 1.2.3.4
```

Útil quando partilham a mesma whitelist (ex: SSH + admin panel restritos
à mesma equipa).

---

## Containers (docker rootless)

`docker.rootless_user` (default `docker24`) é o **único** user com acesso ao
daemon docker. dev24 (operator) **não** corre containers — design intencional.

Para correr containers, três caminhos:

### 1. Como root, escalando para o user (scripts)

```bash
sudo -iu docker24 docker run hello-world
sudo -iu docker24 docker compose -f /home/docker24/stacks/app/compose.yaml up -d
```

`sudo -i` cria login shell, dispara user systemd manager, popula
`XDG_RUNTIME_DIR` — `docker` cliente encontra socket sem mais nada.

### 2. SSH directo como docker24 (interactivo)

A partir desta versão, `lsm docker` pede password ao criar o user, pelo que
podes fazer login directo:

```bash
ssh -p 2210 docker24@<servidor>
docker run hello-world
```

(Servidor antigo onde docker24 ficou sem password? `sudo passwd docker24` arranja.)

### 3. `su -` a partir de root

```bash
su - docker24
docker ps
```

### Pitfalls

- `sudo docker` plain **falha**: `DOCKER_HOST` default aponta a
  `/var/run/docker.sock` (rootful) que está desativado. Tem de ser via login
  do docker user OU `DOCKER_HOST=unix:///run/user/$(id -u docker24)/docker.sock`
  no env.
- Para expor porta do container ao mundo: `sudo lsm port add 3306/tcp "MySQL"`
  e depois `lsm port allow 3306/tcp <IP>` para restringir.
- Compose files vivem tipicamente em `/home/docker24/stacks/<app>/compose.yaml`.

---

## Validate

```bash
sudo lsm validate
```

Audita o estado atual contra config + `state.installed_modules`. Módulos
não instalados pelo lsm aparecem como `[--]` (skipped, não falham). Se
houver FAILs e o caller for **admin**, pergunta se quer re-correr os
módulos com falha para aplicar fixes.

Output exemplo:

```
=== Validate Setup ===
Config: /etc/lsm/config.yaml

  [OK  ] UFW installed
  [OK  ] UFW active
  [OK  ] user 'dev24' exists
  [OK  ] sshd Port = 2210
  [OK  ] sshd PermitRootLogin no
  [OK  ] UFW allows SSH port 2210/tcp
  [OK  ] user 'docker24' exists
  [OK  ] subuid configured — docker24:165536:65536
  [OK  ] Docker engine present
  [OK  ] UFW allows 2210/tcp (SSH)
  [OK  ] fail2ban installed
  [OK  ] fail2ban active
  [OK  ] unattended-upgrades installed
  [OK  ] unattended-upgrades enabled
  [OK  ] NTP enabled (timedatectl)
  [OK  ] system synchronized
  [OK  ] Timezone = Etc/UTC
  [OK  ] sysctl net.ipv4.ip_forward = 1
  [OK  ] sysctl vm.swappiness = 10
  [FAIL] hostname = srv01 — current: debian

Total: 19 OK, 1 FAIL, 0 skipped
```

Exit code != 0 se houver FAIL → útil em pipelines de CI/health check.

---

## Workflow recomendado

### Servidor novo

```bash
# 1. Instala (como root, via console / IPMI)
ARCH=$(dpkg --print-architecture)
rm -f lsm_*.deb
wget -O lsm_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/lsm_${ARCH}.deb
sudo apt install -y ./lsm_${ARCH}.deb

# 2. Bootstrap + wizard
sudo lsm
# Passos automáticos:
# - apt update + upgrade (se reboot needed → prompt, lsm sai, reentras depois)
# - wizard pergunta config + password do dev24 + password do docker24
# - corre módulos baseline + docker (se opt-in)
# - oferece auto-launch do menu no login do dev24

# 3. SESSÃO PARALELA: confirma SSH funciona na nova porta
ssh -p 2210 dev24@<server>

# 4. Remove a porta 22 do UFW (só após confirmar 2210)
sudo lsm port revoke ... # ou manualmente: sudo ufw delete allow 22/tcp

# 5. Valida
sudo lsm validate
```

### Adicionar acesso a colaborador

```bash
# IP único para todas as portas geridas:
sudo lsm add-ip 203.0.113.42

# Ou só para uma porta específica:
sudo lsm port allow 2210/tcp 203.0.113.42
```

### Update + reboot

```bash
sudo lsm system update     # corre apt update + upgrade + autoremove
                            # se serviços/kernel/microcode pendem → pergunta reboot
sudo lsm system reboot     # reboot now / amanhã 04:00 / adiar
```

### Re-correr só um módulo

```bash
sudo lsm fail2ban   # re-aplica jail
sudo lsm validate   # confirma
```

### Correr containers

```bash
sudo -iu docker24 docker run hello-world
# ou ssh directo como docker24, ou su - docker24
```

---

## Troubleshooting

**`lsm: command not found` após reboot**
Já não devia acontecer — wrapper em `/usr/local/bin/lsm` é instalado pelo
`PersistentPreRunE` na primeira invocação como root. Se faltar:
```bash
sudo tee /usr/local/bin/lsm >/dev/null <<'EOF'
#!/bin/sh
exec sudo /usr/sbin/lsm "$@"
EOF
sudo chmod +x /usr/local/bin/lsm
```

**`must run as root`**
Subcomandos exigem root. Sudoers drop-in dá NOPASSWD ao SSH user para
`/usr/sbin/lsm`. Operações destrutivas requerem login direto como root.

**`this operation requires direct root login`**
Estás como dev24 (operator) a tentar correr cmd destrutivo. Login como root
via console/IPMI/`su -`.

**`UFW não instalado`** ao correr `lsm ssh`
Corre primeiro: `sudo lsm firewall`.

**Fiquei trancado fora por SSH**
Acede via consola (DigitalOcean/Hetzner web console, etc). Restaura porta 22:
```bash
sudo ufw allow 22/tcp
sudo systemctl reload-or-restart ssh
```

**Dialog "Which services should be restarted?" durante apt upgrade**
Não devia aparecer. Drop-in em `/etc/needrestart/conf.d/99-lsm.conf` força
modo list-only. Se faltar:
```bash
sudo tee /etc/needrestart/conf.d/99-lsm.conf >/dev/null <<'EOF'
$nrconf{restart} = 'l';
$nrconf{kernelhints} = -1;
$nrconf{ucodehints} = 0;
EOF
```

**`unattended-upgrade` não corre automaticamente**
Verifica timer: `systemctl status apt-daily.timer apt-daily-upgrade.timer`.

**Docker rootless falha "One of slirp4netns ... needs to be installed"**
Faltam runtime deps em Debian minimal:
```bash
sudo apt install -y slirp4netns fuse-overlayfs systemd-container
sudo lsm docker
```

**Config corrompido**
```bash
sudo cp /etc/lsm/config.yaml /etc/lsm/config.yaml.bak
sudo rm /etc/lsm/config.yaml
sudo lsm   # arranca bootstrap
```

---

## Limites conhecidos

- **Só Debian** (12+, testado em 13 trixie). RHEL/Alpine/Arch não suportados.
- **Requer systemd** (timedatectl, systemctl, machinectl, loginctl).
- **Não gere SSH keys** (planeado: módulo `ssh-keys` para deploy de `authorized_keys` + desligar `PasswordAuthentication`).
- **Não gere wireguard, reverse proxy, backups** (módulos futuros).
- **Auto-launch só p/ bash login shells** (lê `~/.profile`). zsh/fish precisaria de drop-in adicional.

---

## Releases

Releases são feitos via GitHub Actions:

1. Vai ao tab **Actions** → **Release (.deb)** → **Run workflow**.
2. Indica `version` (ex: `0.1.0`). Branch é a escolhida no dropdown
   nativo "Use workflow from" (default `master`).
3. Workflow:
   - Faz checkout da branch.
   - Compila linux/amd64 + linux/arm64.
   - Empacota cada um em `.deb` (com postinst).
   - Cria tag `vX.Y.Z`.
   - Publica GitHub Release com os 2 `.deb` anexados + release notes auto-geradas.

---

## Desenvolvimento

```bash
# layout
.
├── main.go
├── go.mod
├── config.example.yaml
├── cmd/                    # subcommands cobra
│   ├── root.go menu.go init.go validate.go all.go ip.go
│   └── firewall.go ssh.go docker.go fail2ban.go upgrades.go timesync.go sysctl.go hostname.go
├── internal/
│   ├── config/             # YAML load/save + validation
│   ├── state/              # state.yaml: managed_ports
│   ├── prompt/             # stdlib prompts
│   ├── runner/             # exec wrapper, dry-run, root check
│   ├── ufw/                # UFW operations
│   ├── ssh/ docker/ fail2ban/ upgrades/ timesync/ sysctl/ hostname/
│   └── ...
└── .github/workflows/release.yml

# build local
go build -o lsm .

# correr (precisa root para ações reais; --dry-run mostra sem executar)
./lsm --help
sudo ./lsm --dry-run all
```

Adicionar módulo novo:
1. `internal/<nome>/<nome>.go` — lógica.
2. `cmd/<nome>.go` — subcommand cobra que chama o módulo.
3. Adiciona ao `chooseAndRunModule()` + `runAllModules()` em `cmd/menu.go`.
4. Adiciona checks ao `cmd/validate.go`.
5. Atualiza este README.

---

## Licença

MIT (a confirmar pelo owner do repo).
