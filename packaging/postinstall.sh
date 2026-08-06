#!/bin/bash
# postinst do pacote erp24 (chamado por dpkg como `postinst configure <old-version>`).
# Distingue install fresco de upgrade e imprime os caminhos relevantes + próximo passo.
# A versão é lida do próprio dpkg (sem placeholder/sed como no antigo release.yml).
set -e

OLD_VERSION="${2:-}"

mkdir -p /etc/erp24
chmod 755 /etc/erp24

# Ficheiros do pacote (binário, config.example.yaml, README) já foram copiados pelo
# dpkg neste ponto. config.yaml e state.yaml não são geridos pelo pacote → preservados.
VERSION="$(dpkg-query -W -f='${Version}' erp24 2>/dev/null || echo '?')"

echo
echo "============================================"
if [ -n "$OLD_VERSION" ]; then
  echo "  erp24 atualizado: ${OLD_VERSION} → ${VERSION}"
else
  echo "  erp24 instalado (versão ${VERSION})"
fi
echo "============================================"
echo "  Binário:    /usr/sbin/erp24"
echo "  Config:     /etc/erp24/config.yaml          (preservado em upgrades)"
echo "  Exemplo:    /etc/erp24/config.example.yaml  (atualizado p/ esta versão)"
echo "  State:      /etc/erp24/state.yaml           (gerido pelo erp24)"
echo "  Docs:       /usr/share/doc/erp24/README.md"
echo

if [ -n "$OLD_VERSION" ]; then
  if [ -f /etc/erp24/config.yaml ]; then
    echo "  Para ver mudanças no schema desde a versão antiga:"
    echo "    diff /etc/erp24/config.yaml /etc/erp24/config.example.yaml"
    echo
    echo "  Validar setup atual:"
    echo "    sudo erp24 validate"
  fi
else
  echo "  Próximo passo:"
  echo "    sudo erp24   (sem config → wizard arranca automaticamente)"
fi
echo
