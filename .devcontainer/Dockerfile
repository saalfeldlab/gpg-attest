FROM node:20

ARG TZ
ENV TZ="$TZ"

ARG CLAUDE_CODE_VERSION=latest

# Install basic development tools and iptables/ipset
RUN apt-get update && apt-get install -y --no-install-recommends \
  aggregate \
  dnsutils \
  fzf \
  gh \
  git \
  gnupg2 \
  iproute2 \
  ipset \
  iptables \
  jq \
  less \
  man-db \
  procps \
  python3 \
  python3-dev \
  python3-pip \
  sudo \
  unzip \
  zip \
  vim \
  xdg-utils \
  default-mysql-server \
  netcat-openbsd \
  redis-server \
  zsh \
  chromium \
  dbus-x11 \
  firefox-esr \
  fonts-dejavu \
  fonts-noto-core \
  libx11-xcb1 \
  librsvg2-bin \
  x11-utils \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Caddy reverse proxy
RUN apt-get update && apt-get install -y --no-install-recommends \
  debian-keyring debian-archive-keyring apt-transport-https && \
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg && \
  curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list && \
  apt-get update && apt-get install -y --no-install-recommends caddy && \
  apt-get clean && rm -rf /var/lib/apt/lists/* && \
  setcap cap_net_bind_service=+ep $(which caddy)

ARG GO_VERSION=1.26.1
RUN ARCH=$(dpkg --print-architecture) && \
  wget -q "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" -O /tmp/go.tar.gz && \
  tar -C /usr/local -xzf /tmp/go.tar.gz && \
  rm /tmp/go.tar.gz

# Ensure default node user has access to /usr/local/share
RUN mkdir -p /usr/local/share/npm-global && \
  chown -R node:node /usr/local/share

ARG USERNAME=node

# Persist bash history.
RUN SNIPPET="export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
  && mkdir /commandhistory \
  && touch /commandhistory/.bash_history \
  && chown -R $USERNAME /commandhistory

# Set `DEVCONTAINER` environment variable to help with orientation
ENV DEVCONTAINER=true

# Create workspace and config directories and set permissions
RUN mkdir -p /workspace /home/node/.claude && \
  chown -R node:node /workspace /home/node/.claude

WORKDIR /workspace

ARG GIT_DELTA_VERSION=0.18.2
RUN ARCH=$(dpkg --print-architecture) && \
  wget "https://github.com/dandavison/delta/releases/download/${GIT_DELTA_VERSION}/git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  sudo dpkg -i "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  rm "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb"

# Python
RUN ln -s /usr/bin/python3 /usr/bin/python

RUN pip3 install --no-cache-dir --break-system-packages "git+https://github.com/github/spec-kit.git"



# Set up non-root user
USER node

# Install global packages
ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin:/usr/local/go/bin:/home/node/go/bin

# Set the default shell to zsh rather than sh
ENV SHELL=/bin/zsh

# Set the default editor and visual
ENV EDITOR vim
ENV VISUAL vim

# Claude Code privacy
ENV CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC 1
ENV CLAUDE_CODE_DISABLE_ANALYTICS 1

# Default powerline10k theme
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN sh -c "$(wget -O- https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh)" -- \
  -p git \
  -p fzf \
  -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
  -a "source /usr/share/doc/fzf/examples/completion.zsh" \
  -a "export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
  -a "zstyle ':completion:*:options' matcher-list ''" \
  -x

RUN go install github.com/google/trillian/cmd/trillian_log_server@latest && \
    go install github.com/google/trillian/cmd/trillian_log_signer@latest && \
    go install github.com/google/trillian/cmd/createtree@latest && \
    go install github.com/swaggo/swag/cmd/swag@latest

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}


# Copy and set up firewall script + Trillian DB init script
COPY init-firewall.sh /usr/local/bin/
COPY init-trillian-db.sh /usr/local/bin/
USER root
RUN chmod +x /usr/local/bin/init-firewall.sh /usr/local/bin/init-trillian-db.sh && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-firewall.sh" > /etc/sudoers.d/node-firewall && \
  chmod 0440 /etc/sudoers.d/node-firewall && \
  echo "node ALL=(root) NOPASSWD: /usr/sbin/service mariadb start" > /etc/sudoers.d/node-mysql && \
  echo "node ALL=(root) NOPASSWD: /usr/local/bin/init-trillian-db.sh" >> /etc/sudoers.d/node-mysql && \
  chmod 0440 /etc/sudoers.d/node-mysql && \
  mkdir -p /usr/local/share/trillian && \
  SCHEMA=$(find /home/node/go/pkg/mod/github.com/google/trillian* \
    -name "storage.sql" -path "*/mysql/schema/*" 2>/dev/null | head -1) && \
  test -n "$SCHEMA" && \
  cp "$SCHEMA" /usr/local/share/trillian/storage.sql
USER node
