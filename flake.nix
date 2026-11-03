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
    in
    {
      packages.${system} = {
        inherit microbot-client;
        default = microbot-client;
      };

      devShells.${system}.default = pkgs.mkShell {
        packages = [
          pkgs.jdk21
          pkgs.gradle
          pkgs.jq
          pkgs.curl
          pkgs.git
        ];
        JAVA_HOME = "${pkgs.jdk21}";
        MICROBOT_VERSION = pin.version;
        MICROBOT_CLIENT_JAR = "${microbot-client}";
        shellHook = ''
          export PATH="$PWD/bin:$PATH"
        '';
      };
    };
}
