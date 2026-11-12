# chronicle-worker-base (docs/11 section 13.4): the Nix runtime of nix/worker-runtime.nix plus the
# plugin jars of `gradle -p microbot pluginJars`, which a pure Nix build cannot see. Built by
# deploy/worker/build-base.sh, which loads the runtime, passes its name as RUNTIME_IMAGE and uses
# microbot/build/plugins as the build context (nothing else goes in).
ARG RUNTIME_IMAGE=chronicle-worker-runtime
FROM ${RUNTIME_IMAGE}
ARG MICROBOT_VERSION
ARG REVISION
COPY *.jar /opt/microbot/plugins/
LABEL org.opencontainers.image.title="chronicle-worker-base" \
      org.opencontainers.image.version="${MICROBOT_VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
