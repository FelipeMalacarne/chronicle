{
  description = "Personal Microbot plugins and the agent dev harness";

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      system = "x86_64-linux";
      pkgs = nixpkgs.legacyPackages.${system};
      pin = builtins.fromJSON (builtins.readFile ./microbot.json);

      # The exact shaded client jar the Microbot launcher runs, pinned by hash.
      microbot-client = pkgs.fetchurl {
        name = "microbot-${pin.version}.jar";
        url = "https://github.com/chsami/Microbot/releases/download/${pin.version}/microbot-${pin.version}.jar";
        inherit (pin) hash;
      };

      # The -javaagent the launcher is meant to attach (device ID / random.dat spoofing).
      hwid-agent = pkgs.fetchurl {
        name = "hwid-agent-${pin.version}.jar";
        url = "https://github.com/chsami/Microbot/releases/download/${pin.version}/hwid-agent-${pin.version}.jar";
        hash = pin.hwidAgentHash;
      };
    in
    {
      packages.${system} = {
        inherit microbot-client hwid-agent;
        default = microbot-client;
      };

      devShells.${system}.default = pkgs.mkShell {
        packages = [
          pkgs.jdk21
          pkgs.gradle
          pkgs.jq
          pkgs.curl
          pkgs.git
          # Java LSP for editors; .nvim.lua enables it in Neovim
          pkgs.jdt-language-server
          # mb-client --headless: own X display + VNC for remote viewing
          pkgs.xvfb
          pkgs.x11vnc
        ];
        JAVA_HOME = "${pkgs.jdk21}";
        # jdtls needs the Lombok agent, or every @Getter/@Slf4j reads as an error.
        JDTLS_JVM_ARGS = "-javaagent:${pkgs.lombok}/share/java/lombok.jar";
        MICROBOT_VERSION = pin.version;
        MICROBOT_CLIENT_JAR = "${microbot-client}";
        MICROBOT_HWID_AGENT_JAR = "${hwid-agent}";
        # GPU / 117 HD load libGL.so.1 (libglvnd), which then picks the system driver from
        # /run/opengl-driver. The launcher's AppImage FHS env provides both; plain java doesn't.
        MICROBOT_LD_LIBRARY_PATH = "${pkgs.libGL}/lib:/run/opengl-driver/lib";
        shellHook = ''
          export PATH="$PWD/bin:$PATH"
        '';
      };
    };
}
