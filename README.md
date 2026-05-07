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

Se não houver `/etc/lsm/config.yaml`:
1. Wizard interativo arranca → pergunta valores (timezone, hostname, SSH user/port/password, docker user, IPs whitelist, política).
2. No fim, podes correr `firewall + ssh + docker + fail2ban + ...` de uma só vez.

Se já existir config:
- Menu mostra: **Validar setup**, **Correr módulo**, **Adicionar IP**, **Remover IP**, **Ver overview**, **Re-init**, **Sair**.

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
| Binário | `/usr/sbin/lsm` | Em PATH para sudo (Debian convention) |
| Config | `/etc/lsm/config.yaml` | **Edita aqui**. Source of truth |
| Exemplo | `/etc/lsm/config.example.yaml` | Template, copia/edita |
| State | `/etc/lsm/state.yaml` | Gerido pelo lsm; portas abertas tracked |
| Docs | `/usr/share/doc/lsm/README.md` | Este ficheiro |

Override do path da config: `sudo lsm --config /caminho/outro.yaml`

---

## Configuração

`/etc/lsm/config.yaml`:

```yaml
timezone: Europe/Lisbon
hostname: srv01            # vazio → módulo hostname não corre
fqdn: srv01.exemplo.com    # opcional

ssh:
  port: 2210
  user: dev24
  password: ALTERA-ME-123

docker:
  rootless_user: docker24

network:
  # Vazia → portas abrem a TODOS (sem restrição de origem)
  # Populada → módulos abrem porta SÓ para estes IPs/CIDRs
  allowed_ips:
    - 10.0.0.5
    - 192.168.1.0/24

  # true | false | ask  — controla quando módulos abrem portas em UFW
  auto_open_ports: ask
```

---

## Comandos

### Top-level

| Comando | Descrição |
|---|---|
| `lsm` | Menu interativo (sem args) |
| `lsm init` | Wizard que cria config |
| `lsm validate` | Audita sistema vs config (read-only) |
| `lsm all` | Corre todos os módulos por ordem |
| `lsm add-ip [IP]` | Adiciona IP/CIDR à whitelist + sincroniza UFW |
| `lsm remove-ip [IP]` | Remove IP/CIDR + sincroniza UFW |

### Módulos

| Comando | Descrição |
|---|---|
| `lsm firewall` | UFW: install, defaults deny/allow, abre porta 22 (bootstrap) |
| `lsm ssh` | Cria user, muda porta sshd, desliga root login, abre porta em UFW |
| `lsm docker` | Remove conflitos, instala Docker, configura rootless num user dedicado |
| `lsm fail2ban` | Instala + jail.local com `port=SSH_PORT` + ignoreip=allowed_ips |
| `lsm upgrades` | Instala unattended-upgrades + ativa periodic |
| `lsm timesync` | Configura systemd-timesyncd + timezone |
| `lsm timesync sync` | Força re-sync NTP imediato |
| `lsm timesync status` | Mostra `timedatectl status` |
| `lsm sysctl` | Escreve `/etc/sysctl.d/99-lsm.conf` (hardening + tuning) + aplica |
| `lsm hostname` | `hostnamectl set-hostname` + atualiza `/etc/hosts` |

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
- Cria user (config `ssh.user` + `ssh.password`), adiciona ao grupo sudo.
- Edita `/etc/ssh/sshd_config`:
  - `Port` = `ssh.port`
  - `PermitRootLogin no`
  - `PasswordAuthentication yes` (até teres ssh-keys)
- Restart sshd.
- Abre nova porta em UFW (respeita `auto_open_ports` + whitelist).
- Regista a porta em `state.yaml` para sincronização futura.

> ⚠️ **Após confirmar SSH na nova porta**, remove a 22:
> ```bash
> sudo ufw delete allow 22/tcp
> ```

### docker
- Remove pacotes conflituosos (`docker.io`, `podman`, etc).
- Instala Docker CE oficial via repo apt.
- Cria user dedicado para containers (config `docker.rootless_user`).
- Configura subuid/subgid + `loginctl enable-linger`.
- Corre `dockerd-rootless-setuptool.sh install` como esse user.
- Desativa Docker rootful (`docker.service` + `docker.socket`).

### fail2ban
- `apt install fail2ban`.
- Escreve `/etc/fail2ban/jail.local`:
  - `bantime=1h`, `findtime=10m`, `maxretry=5`
  - `ignoreip = 127.0.0.1/8 ::1 + allowed_ips`
  - `[sshd]` enabled na porta de `ssh.port`
  - `backend=systemd` (Debian moderno usa journald)
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

## Whitelist de IPs

Lista única em `network.allowed_ips` controla **acesso** às portas geridas pelo lsm.

- **Vazia** → quando módulo abre porta, abre a TODOS (`ufw allow PORT/tcp`).
- **Populada** → quando módulo abre porta, só esses IPs podem ligar
  (`ufw allow from IP to any port PORT proto tcp`, uma regra por IP).

### Adicionar IP em produção

```bash
sudo lsm add-ip 1.2.3.4
```

Ações:
1. Adiciona à `network.allowed_ips` no `config.yaml`.
2. Para cada porta em `state.yaml` (managed_ports): `ufw allow from 1.2.3.4 to any port X proto Y`.
3. Se a whitelist passou de 0 para 1 IP, remove as regras "todos" das portas geridas.

### Remover IP

```bash
sudo lsm remove-ip 1.2.3.4
```

Ações:
1. Remove de `config.yaml`.
2. Para cada porta gerida: `ufw delete allow from 1.2.3.4 to any port X proto Y`.
3. Se ficou whitelist vazia, reabre as portas geridas a TODOS.

---

## Validate

```bash
sudo lsm validate
```

Audita o estado atual contra o config. Output exemplo:

```
=== Validar Setup ===
Config: /etc/lsm/config.yaml

  [OK  ] UFW instalado
  [OK  ] UFW ativo
  [OK  ] user 'dev24' existe
  [OK  ] sshd Port = 2210
  [OK  ] sshd PermitRootLogin no
  [OK  ] UFW permite porta SSH 2210/tcp
  [OK  ] user 'docker24' existe
  [OK  ] subuid configurado — docker24:100000:65536
  [OK  ] Docker engine presente
  [OK  ] UFW permite 2210/tcp (SSH)
  [OK  ] fail2ban instalado
  [OK  ] fail2ban ativo
  [OK  ] unattended-upgrades instalado
  [OK  ] unattended-upgrades enabled
  [OK  ] NTP enabled (timedatectl)
  [OK  ] Sistema sincronizado
  [OK  ] Timezone = Europe/Lisbon
  [OK  ] sysctl net.ipv4.ip_forward = 1
  [OK  ] sysctl vm.swappiness = 10
  [FAIL] hostname = srv01 — atual: debian

Total: 19 OK, 1 FAIL
```

Exit code != 0 se houver FAIL → útil em pipelines de CI/health check.

---

## Workflow recomendado

### Servidor novo

```bash
# 1. Instala
sudo dpkg -i lsm_X.Y.Z_amd64.deb

# 2. Wizard + corre tudo
sudo lsm
# (responde às perguntas, escolhe "all" no fim)

# 3. Confirma SSH funciona na nova porta numa SESSÃO PARALELA
ssh -p 2210 dev24@<server>

# 4. Remove a porta 22 do UFW
sudo ufw delete allow 22/tcp

# 5. Valida
sudo lsm validate
```

### Adicionar acesso a colaborador

```bash
sudo lsm add-ip 203.0.113.42
```

Pronto. Sincroniza todas as portas geridas.

### Re-correr só um módulo

```bash
sudo lsm fail2ban   # re-aplica jail (após editar config)
sudo lsm validate   # confirma
```

---

## Troubleshooting

**`lsm: command not found`**
`/usr/sbin` não está no PATH do user normal. Usa `sudo lsm` (sudo inclui sbin).

**`must run as root`**
Sempre `sudo lsm`. Subcomandos read-only (`validate`, `timesync status`) também precisam de root para algumas verificações.

**`UFW não instalado`** ao correr `lsm ssh`
Corre primeiro: `sudo lsm firewall`.

**Fiquei trancado fora por SSH**
Acede via consola (DigitalOcean/Hetzner web console, etc). Restaura porta 22:
```bash
sudo ufw allow 22/tcp
sudo systemctl restart sshd
```

**`unattended-upgrade` não corre automaticamente**
Verifica timer: `systemctl status apt-daily.timer apt-daily-upgrade.timer`.

**Config corrompido**
```bash
sudo cp /etc/lsm/config.yaml /etc/lsm/config.yaml.bak
sudo rm /etc/lsm/config.yaml
sudo lsm   # arranca wizard
```

---

## Limites conhecidos

- **Só Debian** (12+). RHEL/Alpine/Arch não suportados.
- **Requer systemd** (timedatectl, systemctl, machinectl, loginctl).
- **Não gere SSH keys** (planeado: módulo `ssh-keys` para deploy de `authorized_keys` + desligar PasswordAuth).
- **Não gere swap, WireGuard, reverse proxy, backups** (módulos futuros).

---

## Releases

Releases são feitos via GitHub Actions:

1. Vai ao tab **Actions** → **Release (.deb)** → **Run workflow**.
2. Indica `version` (ex: `0.1.0`) + `branch` (default `main`).
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
