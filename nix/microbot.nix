# The Microbot release the plugins compile against and the workers run, pinned by
# microbot/microbot.json (`mb-bump <version>` rewrites it).
{ fetchurl }:
let
  pin = builtins.fromJSON (builtins.readFile ../microbot/microbot.json);
  release = "https://github.com/chsami/Microbot/releases/download/${pin.version}";
in
{
  inherit pin;

  # The exact shaded client jar the Microbot launcher runs.
  microbot-client = fetchurl {
    name = "microbot-${pin.version}.jar";
    url = "${release}/microbot-${pin.version}.jar";
    inherit (pin) hash;
  };

  # The -javaagent the launcher attaches (device ID / random.dat spoofing).
  hwid-agent = fetchurl {
    name = "hwid-agent-${pin.version}.jar";
    url = "${release}/hwid-agent-${pin.version}.jar";
    hash = pin.hwidAgentHash;
  };

  # The upstream CLI of the same tag (`mb`), for the worker image, which has no vendor/.
  microbot-cli = fetchurl {
    name = "microbot-cli-${pin.version}";
    url = "https://raw.githubusercontent.com/chsami/Microbot/${pin.version}/microbot-cli";
    hash = pin.cliHash;
  };
}
