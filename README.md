# erp24 — Linux Server Manager

Provisionamento idempotente de servidores **Debian** num único binário Go.
Substitui dezenas de scripts bash dispersos por uma CLI com menu interativo
e subcomandos para CI/automação.

> Só Debian (testado em 12+). Requer `systemd`.

---

## Porquê

Configurar um servidor novo envolve sempre os mesmos passos: firewall,
SSH hardening, Docker, fail2ban, security updates, timezone, sysctl, hostname.
Tipicamente há um script bash gigante por equipa, frágil e não-reversível.

`erp24` resolve isso com:

- **Idempotência**: cada módulo verifica estado antes de agir, podes correr 10×.
- **Modular**: corres só o que precisas (`erp24 fail2ban`) ou tudo (`erp24 all`).
- **Validation**: `erp24 validate` audita o sistema vs config sem mudar nada.
- **State-aware**: portas abertas pelo erp24 ficam tracked; adicionar/remover
  IPs da whitelist sincroniza UFW automaticamente.
- **Único binário Go**: zero deps Python/Ruby; cross-compile fácil.
- **Interativo por defeito**: corres `sudo erp24`, segues o menu.

---

## Instalar

> Repo: **https://github.com/santospedro1993/LittleServerManager**
> Releases: **https://github.com/santospedro1993/LittleServerManager/releases**

### Via .deb (recomendado) — sempre a versão mais recente

Detecta arch automaticamente (`amd64` ou `arm64`):

```bash
ARCH=$(dpkg --print-architecture)
rm -f erp24_*.deb                                   # limpa downloads antigos
wget -O erp24_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/erp24_${ARCH}.deb
sudo apt install -y ./erp24_${ARCH}.deb
sudo erp24
```

> ⚠️ `wget -O` força sobrescrever o ficheiro. Sem isso, `wget` cria
> `erp24_amd64.deb.1` em re-downloads e o `apt install` instala o
> **antigo** que continua em disco. Por isso o `rm -f` antes.

> Usar `apt install ./file.deb` em vez de `dpkg -i` resolve dependências
> automaticamente.

### Versão específica

```bash
VER=0.1.0
ARCH=$(dpkg --print-architecture)
rm -f erp24_*.deb
wget -O erp24_${VER}_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/download/v${VER}/erp24_${VER}_${ARCH}.deb
sudo apt install -y ./erp24_${VER}_${ARCH}.deb
```

A instalação cria `/etc/erp24/` e imprime os caminhos relevantes + próximo passo.

### Build a partir do código

```bash
git clone https://github.com/USER/erp24.git
cd erp24
go build -o erp24 .
sudo install -m 755 erp24 /usr/sbin/erp24
sudo mkdir -p /etc/erp24
sudo install -m 644 config.example.yaml /etc/erp24/config.example.yaml
```

Requer Go 1.23+.

### Build .deb localmente

```bash
GOOS=linux GOARCH=amd64 go build -o erp24 .
# repete o bloco "Build .deb" do .github/workflows/release.yml
```

---

## Quick start

```bash
sudo erp24
```

**Primeiro arranque** (sem `/etc/erp24/config.yaml`):
1. **Apt update + upgrade** automático antes de tocar em config (kernel/security
   patches primeiro). Se for preciso reboot, erp24 pergunta e sai — re-corres
   depois e retoma daqui.
2. **Wizard** pergunta valores: timezone (default `Etc/UTC`), hostname, SSH
   user/port, política `auto_open_ports`. Modules baseline (firewall, ssh,
   sysctl, timesync, hostname, fail2ban, upgrades) são sempre instalados;
   **só docker é opt-in**.
3. **Password** do user SSH é pedida em runtime (`stty -echo` +
   double-prompt). Não fica em ficheiro.
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
rm -f erp24_*.deb                                   # crítico: senão apt instala o antigo
wget -O erp24_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/erp24_${ARCH}.deb
sudo apt install -y ./erp24_${ARCH}.deb
```

O que acontece em cada upgrade:

| Ficheiro | Ação |
|---|---|
| `/usr/sbin/erp24` | **Substituído** pela nova versão |
| `/etc/erp24/config.example.yaml` | **Atualizado** (reflete schema novo) |
| `/etc/erp24/config.yaml` | **Preservado** (a tua config fica intacta) |
| `/etc/erp24/state.yaml` | **Preservado** (portas geridas tracked) |
| `/usr/share/doc/erp24/README.md` | Atualizado |

O `postinst` deteta upgrade e mostra:
- Versão antiga → nova
- Comando de diff: `diff /etc/erp24/config.yaml /etc/erp24/config.example.yaml`
- Sugere `sudo erp24 validate` para confirmar que tudo continua coerente

### Comparar versão atual vs latest publicada

```bash
erp24 --version
curl -s https://api.github.com/repos/santospedro1993/LittleServerManager/releases/latest | grep tag_name
```

### Após upgrade — validar

```bash
sudo erp24 validate
```

Se o schema mudou e a tua config tem campos novos em falta, **erp24 vai usar
defaults sensatos**. Para incorporares manualmente:

```bash
sudo diff /etc/erp24/config.yaml /etc/erp24/config.example.yaml
sudo $EDITOR /etc/erp24/config.yaml
sudo erp24 validate
```

### Downgrade

Funciona via `apt install` apontando para `.deb` mais antigo. Mas o `state.yaml`
pode ter formato mais novo — re-correr módulos regenera-o.

### Remover

```bash
sudo apt remove erp24        # tira binário, mantém /etc/erp24
sudo apt purge erp24         # mesma coisa (não há conffiles)
sudo rm -rf /etc/erp24       # remoção total da config + state
```

---

## Localizações

| O quê | Onde | Notas |
|---|---|---|
| Binário real | `/usr/sbin/erp24` | Convenção Debian para tools que precisam de root |
| Wrapper PATH | `/usr/local/bin/erp24` | `exec sudo /usr/sbin/erp24 "$@"` — `erp24` plain funciona p/ user não-root |
| Config | `/etc/erp24/config.yaml` | **Edita aqui**. Source of truth |
| Exemplo | `/etc/erp24/config.example.yaml` | Template, copia/edita |
| State | `/etc/erp24/state.yaml` | Gerido pelo erp24; portas geridas + módulos instalados |
| sshd drop-in | `/etc/ssh/sshd_config.d/99-erp24.conf` | Port + PermitRootLogin no + PasswordAuthentication yes |
| Sudoers | `/etc/sudoers.d/erp24` | NOPASSWD blanket em `/usr/sbin/erp24` para SSH user (gate em-app) |
| needrestart | `/etc/needrestart/conf.d/99-erp24.conf` | Modo list-only (sem dialog interativo no apt) |
| Auto-launch | `~/.profile` (SSH user) | Bloco marcado: corre `sudo erp24` no login interativo |
| Docs | `/usr/share/doc/erp24/README.md` | Este ficheiro |

Override do path da config: `sudo erp24 --config /caminho/outro.yaml`

---

## Configuração

`/etc/erp24/config.yaml` representa **intenção** — sem segredos, sem dados
derivados (whitelist de IPs vem do estado real do UFW; passwords são pedidas
em runtime).

```yaml
timezone: Etc/UTC
hostname: srv01                  # vazio → módulo hostname não corre
fqdn: srv01.example.com          # opcional

ssh:
  port: 2210                     # alternativa à 22 reduz brute-force
  user: dev24                    # password pedida ao criar user

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
| `erp24` | Menu interativo |
| `erp24 init` | Wizard de config |
| `erp24 validate` | Audita sistema vs config (read-only) |
| `erp24 all` | Corre módulos selecionados em `config.modules.*` |
| `erp24 status [--live]` | CPU / RAM / disk / network. `--live` faz refresh 2s |
| `erp24 system update` | apt update + upgrade + autoremove (sem restart inline; pergunta reboot) |
| `erp24 system reboot` | Reboot agora / agendar 04:00 / adiar |
| `erp24 port add P/PROTO [LABEL]` | Registar + abrir porta. Default kind=docker (`ufw route allow`); `--host` para serviço host (`ufw allow`); `--restrict` regista sem abrir |
| `erp24 port remove P/PROTO` | Fechar + remover de state |
| `erp24 port allow P/PROTO IP` | ALLOW from IP só nessa porta (chain dispatched by kind) |
| `erp24 port revoke P/PROTO IP` | DELETE allow from IP só nessa porta |
| `erp24 port list` | Tabela: portas geridas, kind, sources UFW |
| `erp24 add-ip [IP]` | Atalho: ALLOW from IP em **todas** portas geridas |
| `erp24 remove-ip [IP]` | Atalho: DELETE em todas portas geridas |

### Módulos

| Comando | Descrição |
|---|---|
| `erp24 firewall` | UFW: install, defaults deny/allow, abre 22 (bootstrap só na 1ª vez) |
| `erp24 ssh` | Cria user (pede password), drop-in `sshd_config.d/99-erp24.conf`, abre porta UFW |
| `erp24 docker` | Instala Docker CE (engine + cli + containerd + buildx + compose plugin) e `systemctl enable --now docker.service`. Daemon corre como root. |
| `erp24 fail2ban` | jail.local com `port=ssh.port` + ignoreip lido do UFW |
| `erp24 upgrades` | unattended-upgrades + periodic config |
| `erp24 timesync` | systemd-timesyncd + timezone |
| `erp24 timesync sync` | Força re-sync NTP |
| `erp24 timesync status` | `timedatectl status` |
| `erp24 sysctl` | Escreve `/etc/sysctl.d/99-erp24.conf` |
| `erp24 hostname` | `hostnamectl set-hostname` + `/etc/hosts` |

### Modelo de permissões

| Role | Como detectado | Pode |
|---|---|---|
| **admin** | uid 0 + (`$SUDO_USER` vazio ou `root`) | tudo |
| **operator** | uid 0 + `$SUDO_USER` é user não-root (ex: dev24) | só read-only + system update/reboot |

Operator é tipicamente o SSH user (default `dev24`) que **não** está no grupo
`sudo`. Privilégio vem só da entrada NOPASSWD em `/etc/sudoers.d/erp24`; erp24
filtra por subcmd em-app. Operações destrutivas precisam de root login
(console / IPMI / `su -`).

### Flags globais

| Flag | Descrição |
|---|---|
| `--config <path>` | Caminho do config file (default `/etc/erp24/config.yaml`) |
| `--dry-run` | Loga ações sem executar |
| `-y, --yes` | Auto-confirma prompts (CI-friendly) |

---

## Como funcionam os módulos

### firewall
- Instala UFW se faltar.
- `default deny incoming / allow outgoing`.
- Abre porta 22 (bootstrap, evita lockout antes do SSH module mudar a porta).
- Ativa UFW.
- Escreve bloco marcado **DOCKER-USER** em `/etc/ufw/after.rules` para que
  portas docker-publicadas respeitem regras UFW (sem isto, `docker run -p`
  bypassa o firewall). `ufw reload` aplica.
- **Idempotente**: re-correr não duplica regras nem o bloco DOCKER-USER.

### ssh
- Cria user (`ssh.user`) + pede password em runtime se ainda não existir.
- **Não** adiciona ao grupo sudo (modelo operator-only).
- Drop-in `/etc/ssh/sshd_config.d/99-erp24.conf`:
  - `Port` = `ssh.port`
  - `PermitRootLogin no`
  - `PasswordAuthentication yes` (até teres ssh-keys)
- Valida com `sshd -t` antes de `systemctl reload-or-restart ssh`.
- Abre nova porta em UFW respeitando `auto_open_ports`.
- Regista porta em `state.yaml`.
- Grava `/etc/sudoers.d/erp24` com NOPASSWD em `/usr/sbin/erp24` (gate em-app).
- Wrapper `/usr/local/bin/erp24` é criado pelo `PersistentPreRunE` (idempotente).

> ⚠️ **Após confirmar SSH na nova porta**, remove a 22:
> ```bash
> sudo ufw delete allow 22/tcp
> ```

### docker
- Skip `RemoveConflicts` se docker já instalado (evita destruir engine ativo).
- Caso contrário remove `docker.io`, `podman`, `runc` (não toca em docker-ce*).
- Instala Docker CE oficial via repo apt: `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`, `docker-compose-plugin`.
- `systemctl enable --now docker.service` — daemon rootful, socket em `/var/run/docker.sock` (só root).
- **Não** cria user dedicado nem adiciona ninguém ao grupo `docker` (= root). Containers correm via root login / `sudo -i`.

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
- `erp24 timesync sync` faz `systemctl restart systemd-timesyncd` (force re-sync).

### sysctl
- Escreve `/etc/sysctl.d/99-erp24.conf` com:
  - **Network hardening**: `rp_filter`, `accept_redirects=0`, `tcp_syncookies`, anti-smurf, `log_martians`.
  - **Forwarding**: `net.ipv4.ip_forward=1` (Docker).
  - **TCP perf**: `somaxconn=1024`, keepalive tuning.
  - **VM**: `vm.swappiness=10`.
  - **fs**: `inotify.max_user_watches=524288` (node/webpack), `file-max`.
- Aplica com `sysctl --system`.
- Re-correr sobrescreve **só** o ficheiro `99-erp24.conf` — não toca em outros sysctl drops.

### hostname
- `hostnamectl set-hostname <config.hostname>`.
- Atualiza `/etc/hosts`:
  - Garante linha `127.0.0.1 localhost`.
  - Substitui ou adiciona linha `127.0.1.1 <fqdn> <hostname>` (Debian convention).
- Skip se `config.hostname` vazio.

---

## Portas + whitelist de IPs

**Source of truth = UFW**. Config NÃO armazena IPs nem portas. erp24 parsea
`ufw status` para saber estado real, e `state.yaml::managed_ports` é só
a lista de portas que o erp24 gere.

Cada porta gerida tem o seu próprio set de IPs permitidos. Estados possíveis:

- **Aberta a todos**: regra `Anywhere` no UFW (sem `from <ip>`).
- **Restrita**: 1+ regras `ALLOW from <ip> to any port P proto T`, e SEM `Anywhere`.

### Por porta (recomendado)

```bash
sudo erp24 port add 3306/tcp "MySQL"            # abre a todos + regista
sudo erp24 port allow 3306/tcp 1.2.3.4          # restringe só a esse IP
sudo erp24 port allow 3306/tcp 192.168.1.0/24   # adiciona outro
sudo erp24 port revoke 3306/tcp 1.2.3.4         # tira IP (reabre a todos se ficou vazio)
sudo erp24 port remove 3306/tcp                 # fecha + remove de state
sudo erp24 port list                            # tabela de estado
```

### Atalho global (aplica a todas as portas geridas)

```bash
sudo erp24 add-ip 1.2.3.4
sudo erp24 remove-ip 1.2.3.4
```

Útil quando partilham a mesma whitelist (ex: SSH + admin panel restritos
à mesma equipa).

---

## Containers (docker rootful)

Daemon docker corre como root, socket em `/var/run/docker.sock` (só root tem
acesso). dev24 (operator) **não** corre containers — único caminho é root
login (console / IPMI / `su -`).

### Caminho típico

```bash
sudo -i                                    # ou root login direto
docker run hello-world
docker compose -f /root/stacks/app/compose.yaml up -d
```

### Expor porta de container

Docker rootful escreve regras directamente em iptables, normalmente
**bypassing** UFW. Para que `erp24 port` se aplique a containers, `erp24
firewall` instala um bloco DOCKER-USER em `/etc/ufw/after.rules`:

```
DOCKER-USER → ufw-user-forward (regras "ufw route allow ...")
              private LAN (10/8, 172.16/12, 192.168/16) → RETURN
              tudo o resto                              → DROP
```

Resultado: container que faz `-p 5432:5432` está **fechado** ao mundo
até dares regra explícita.

Fluxo típico:

```bash
docker run -d -p 5432:5432 postgres:16
# (porta fechada ao mundo — DOCKER-USER DROP por defeito)

sudo erp24 port add 5432/tcp postgres           # kind=docker (default)
sudo erp24 port allow 5432/tcp 1.2.3.4          # só 1.2.3.4 entra
```

### Pitfalls

- **Não** adiciones users ao grupo `docker`. É equivalente a root sem audit
  trail. Se um operador precisa de docker, dá-lhe root login mesmo.
- Para serviço **host** (não-container) que escuta no host, usa
  `sudo erp24 port add 8080/tcp "label" --host` para criar regra INPUT em vez
  de FORWARD.
- Compose files vivem tipicamente em `/root/stacks/<app>/compose.yaml`
  (ou `/opt/stacks/`).

---

## Validate

```bash
sudo erp24 validate
```

Audita o estado atual contra config + `state.installed_modules`. Módulos
não instalados pelo erp24 aparecem como `[--]` (skipped, não falham). Se
houver FAILs e o caller for **admin**, pergunta se quer re-correr os
módulos com falha para aplicar fixes.

Output exemplo:

```
=== Validate Setup ===
Config: /etc/erp24/config.yaml

  [OK  ] UFW installed
  [OK  ] UFW active
  [OK  ] user 'dev24' exists
  [OK  ] sshd Port = 2210
  [OK  ] sshd PermitRootLogin no
  [OK  ] UFW allows SSH port 2210/tcp
  [OK  ] Docker engine present
  [OK  ] docker.service active
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
rm -f erp24_*.deb
wget -O erp24_${ARCH}.deb \
  https://github.com/santospedro1993/LittleServerManager/releases/latest/download/erp24_${ARCH}.deb
sudo apt install -y ./erp24_${ARCH}.deb

# 2. Bootstrap + wizard
sudo erp24
# Passos automáticos:
# - apt update + upgrade (se reboot needed → prompt, erp24 sai, reentras depois)
# - wizard pergunta config + password do dev24
# - corre módulos baseline + docker (se opt-in)
# - oferece auto-launch do menu no login do dev24

# 3. SESSÃO PARALELA: confirma SSH funciona na nova porta
ssh -p 2210 dev24@<server>

# 4. Remove a porta 22 do UFW (só após confirmar 2210)
sudo erp24 port revoke ... # ou manualmente: sudo ufw delete allow 22/tcp

# 5. Valida
sudo erp24 validate
```

### Adicionar acesso a colaborador

```bash
# IP único para todas as portas geridas:
sudo erp24 add-ip 203.0.113.42

# Ou só para uma porta específica:
sudo erp24 port allow 2210/tcp 203.0.113.42
```

### Update + reboot

```bash
sudo erp24 system update     # corre apt update + upgrade + autoremove
                            # se serviços/kernel/microcode pendem → pergunta reboot
sudo erp24 system reboot     # reboot now / amanhã 04:00 / adiar
```

### Re-correr só um módulo

```bash
sudo erp24 fail2ban   # re-aplica jail
sudo erp24 validate   # confirma
```

### Correr containers

```bash
sudo -i
docker run hello-world
```

---

## Troubleshooting

**`erp24: command not found` após reboot**
Já não devia acontecer — wrapper em `/usr/local/bin/erp24` é instalado pelo
`PersistentPreRunE` na primeira invocação como root. Se faltar:
```bash
sudo tee /usr/local/bin/erp24 >/dev/null <<'EOF'
#!/bin/sh
exec sudo /usr/sbin/erp24 "$@"
EOF
sudo chmod +x /usr/local/bin/erp24
```

**`must run as root`**
Subcomandos exigem root. Sudoers drop-in dá NOPASSWD ao SSH user para
`/usr/sbin/erp24`. Operações destrutivas requerem login direto como root.

**`this operation requires direct root login`**
Estás como dev24 (operator) a tentar correr cmd destrutivo. Login como root
via console/IPMI/`su -`.

**`UFW não instalado`** ao correr `erp24 ssh`
Corre primeiro: `sudo erp24 firewall`.

**Fiquei trancado fora por SSH**
Acede via consola (DigitalOcean/Hetzner web console, etc). Restaura porta 22:
```bash
sudo ufw allow 22/tcp
sudo systemctl reload-or-restart ssh
```

**Dialog "Which services should be restarted?" durante apt upgrade**
Não devia aparecer. Drop-in em `/etc/needrestart/conf.d/99-erp24.conf` força
modo list-only. Se faltar:
```bash
sudo tee /etc/needrestart/conf.d/99-erp24.conf >/dev/null <<'EOF'
$nrconf{restart} = 'l';
$nrconf{kernelhints} = -1;
$nrconf{ucodehints} = 0;
EOF
```

**`unattended-upgrade` não corre automaticamente**
Verifica timer: `systemctl status apt-daily.timer apt-daily-upgrade.timer`.

**`docker.service` falha após install**
Verifica `journalctl -u docker.service -n 50`. Causas comuns: iptables
legacy ausente, kernel sem cgroup v2, conflito com pacote antigo do
distro. Re-correr `sudo erp24 docker` re-instala via repo oficial.

**Config corrompido**
```bash
sudo cp /etc/erp24/config.yaml /etc/erp24/config.yaml.bak
sudo rm /etc/erp24/config.yaml
sudo erp24   # arranca bootstrap
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
go build -o erp24 .

# correr (precisa root para ações reais; --dry-run mostra sem executar)
./erp24 --help
sudo ./erp24 --dry-run all
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
