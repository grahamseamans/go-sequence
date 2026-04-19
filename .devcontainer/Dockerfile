FROM node:20

ARG TZ
ENV TZ="$TZ"

ARG CLAUDE_CODE_VERSION=latest
ARG GOLANG_VERSION=1.25.4

# Install basic development tools, iptables/ipset, and C/C++ build deps for
# gomidi's rtmidi C++ driver (libasound2-dev = ALSA headers for Linux MIDI;
# libjack-jackd2-dev = JACK headers; gcc/g++/pkg-config/make to compile).
# MIDI I/O won't actually reach hardware from Docker on macOS, but the build
# must succeed. Tests run headlessly against MockSurface.
RUN apt-get update && apt-get install -y --no-install-recommends \
  less \
  git \
  procps \
  sudo \
  fzf \
  zsh \
  man-db \
  unzip \
  gnupg2 \
  gh \
  jq \
  nano \
  vim \
  curl \
  build-essential \
  pkg-config \
  libasound2-dev \
  libjack-jackd2-dev \
  python3 \
  python3-venv \
  ca-certificates \
  && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install uv (Python package/project manager). System-wide so `uv` is on
# PATH for all users. Needed for Python-based MCPs (digikey) and general
# Python scripting per the python-runtime skill.
RUN curl -LsSf https://astral.sh/uv/install.sh | \
  env UV_INSTALL_DIR=/usr/local/bin UV_UNMANAGED_INSTALL=/usr/local/bin sh

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

# Create workspace, Claude config, and Go cache directories. Permissions set
# so the node user can read/write without sudo.
RUN mkdir -p /workspace /home/node/.claude /home/node/go/pkg/mod /home/node/.cache/go-build && \
  chown -R node:node /workspace /home/node/.claude /home/node/go /home/node/.cache

WORKDIR /workspace

ARG GIT_DELTA_VERSION=0.18.2
RUN ARCH=$(dpkg --print-architecture) && \
  wget "https://github.com/dandavison/delta/releases/download/${GIT_DELTA_VERSION}/git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  sudo dpkg -i "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb" && \
  rm "git-delta_${GIT_DELTA_VERSION}_${ARCH}.deb"

# Install Go toolchain. Match project's go.mod version.
# Supports both amd64 and arm64 (Apple Silicon Docker Desktop uses arm64).
RUN ARCH=$(dpkg --print-architecture) && \
  case "$ARCH" in \
    amd64) GO_ARCH=amd64 ;; \
    arm64) GO_ARCH=arm64 ;; \
    *) echo "Unsupported architecture: $ARCH" && exit 1 ;; \
  esac && \
  wget -O /tmp/go.tar.gz "https://go.dev/dl/go${GOLANG_VERSION}.linux-${GO_ARCH}.tar.gz" && \
  tar -C /usr/local -xzf /tmp/go.tar.gz && \
  rm /tmp/go.tar.gz

ENV PATH=$PATH:/usr/local/go/bin:/home/node/go/bin

# Set up non-root user
USER node

# Install global npm packages
ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin

# Set the default shell to zsh rather than sh
ENV SHELL=/bin/zsh

# Set the default editor and visual
ENV EDITOR=nano
ENV VISUAL=nano

# Default powerline10k theme
ARG ZSH_IN_DOCKER_VERSION=1.2.0
RUN sh -c "$(wget -O- https://github.com/deluan/zsh-in-docker/releases/download/v${ZSH_IN_DOCKER_VERSION}/zsh-in-docker.sh)" -- \
  -p git \
  -p fzf \
  -a "source /usr/share/doc/fzf/examples/key-bindings.zsh" \
  -a "source /usr/share/doc/fzf/examples/completion.zsh" \
  -a "export PROMPT_COMMAND='history -a' && export HISTFILE=/commandhistory/.bash_history" \
  -x

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}
