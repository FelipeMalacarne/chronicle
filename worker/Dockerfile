# microbot-worker-base (planner docs/11 section 13.4): the Nix runtime of worker/image.nix plus the
# plugin jars of `gradle pluginJars`, which a pure Nix build cannot see. Build it with
# worker/build.sh, which loads the runtime and passes its name as RUNTIME_IMAGE.
ARG RUNTIME_IMAGE=microbot-worker-runtime
FROM ${RUNTIME_IMAGE}
ARG MICROBOT_VERSION
ARG REVISION
COPY build/plugins/*.jar /opt/microbot/plugins/
LABEL org.opencontainers.image.title="microbot-worker-base" \
      org.opencontainers.image.version="${MICROBOT_VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
