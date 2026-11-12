# Runtime of the microbot-worker-base image (planner docs/11 section 13.4): everything the
# headless client needs, from the same nixpkgs as the dev shell (a java from another nixpkgs
# breaks on glibc). The plugin jars are not here: a pure Nix build cannot see gradle's output,
# so worker/Dockerfile adds build/plugins/*.jar on top (worker/build.sh does both).
{
  pkgs,
  pin,
  microbot-client,
  hwid-agent,
  microbot-cli,
}:
let
  uid = "65532";

  # The harness under /opt/microbot: bin/ (on PATH), microbot.json and the pinned microbot-cli.
  harness = pkgs.runCommand "microbot-harness-${pin.version}" { } ''
    mkdir -p $out/opt/microbot/libexec $out/opt/microbot/plugins
    cp -r ${../bin} $out/opt/microbot/bin
    chmod -R u+w,a+rx $out/opt/microbot/bin
    cp ${../microbot.json} $out/opt/microbot/microbot.json
    install -m755 ${microbot-cli} $out/opt/microbot/libexec/microbot-cli
  '';

  # The dev shell's JDK itself. A jlink JRE of it came out bigger, not smaller: its files keep
  # store paths of the JDK, so the image carried both.
  jre = pkgs.jdk21;

  # Java's font lookup goes through fontconfig, and the client draws text before any login.
  fontsConf = pkgs.makeFontsConf { fontDirectories = [ pkgs.dejavu_fonts ]; };

  passwd = pkgs.writeTextDir "etc/passwd" ''
    root:x:0:0:root:/root:/bin/bash
    worker:x:${uid}:${uid}:worker:/home/worker:/bin/bash
    nobody:x:65534:65534:nobody:/var/empty:/bin/false
  '';
  group = pkgs.writeTextDir "etc/group" ''
    root:x:0:
    worker:x:${uid}:
    nobody:x:65534:
  '';
in
pkgs.dockerTools.streamLayeredImage {
  name = "microbot-worker-runtime";
  contents = [
    harness
    passwd
    group
    pkgs.dockerTools.usrBinEnv
    pkgs.dockerTools.binSh
    pkgs.dockerTools.caCertificates
    jre
    pkgs.bashInteractive
    pkgs.coreutils
    pkgs.util-linux # setsid, flock
    pkgs.procps
    pkgs.gnugrep
    pkgs.gnused
    pkgs.gawk # microbot-cli
    pkgs.findutils
    pkgs.jq
    pkgs.curl
    pkgs.xvfb
    pkgs.x11vnc
    pkgs.xdotool
    pkgs.fontconfig
    pkgs.dejavu_fonts
    pkgs.cacert
  ];
  # /tmp for Xvfb's /tmp/.X11-unix; the home exists and belongs to the worker, so the image also
  # runs without a volume there (the pod mounts an emptyDir on it).
  fakeRootCommands = ''
    mkdir -p tmp home/worker/microbot-dev run
    chmod 1777 tmp
    chown -R ${uid}:${uid} home/worker
  '';
  config = {
    User = "${uid}:${uid}";
    WorkingDir = "/home/worker";
    Env = [
      "PATH=/opt/microbot/bin:/bin:/usr/bin"
      "HOME=/home/worker"
      "USER=worker"
      "LANG=C.UTF-8"
      "MB_DEV_HOME=/home/worker/microbot-dev"
      "MB_PREBUILT_PLUGINS=/opt/microbot/plugins"
      "MICROBOT_VERSION=${pin.version}"
      "MICROBOT_CLIENT_JAR=${microbot-client}"
      "MICROBOT_HWID_AGENT_JAR=${hwid-agent}"
      "MICROBOT_CLI=/opt/microbot/libexec/microbot-cli"
      "JAVA_HOME=${jre}"
      "FONTCONFIG_FILE=${fontsConf}"
      "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
    ];
    Labels = {
      "org.opencontainers.image.source" = "https://github.com/FelipeMalacarne/microbot-plugins";
      "org.opencontainers.image.description" = "Headless Microbot ${pin.version} runtime for Chronicle workers";
    };
  };
}
