# Home Manager module for herdr-preview-grid.
#
# Nix owns the plugin registration: the built package is linked into herdr's
# plugin registry from an immutable store path via `herdr plugin link` at
# activation. No Go toolchain and no network are needed on the target machine
# (contrast `herdr plugin install`, which clones from GitHub and runs the
# manifest's [[build]] with whatever toolchain it finds on PATH).
#
# Keys are NOT managed here — herdr's plugin manifest has no key schema, so
# keybindings belong in the user's herdr config.toml as
# `type = "plugin_action"` entries (see the plugin README).
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.herdrPreviewGrid;
  manifest = builtins.fromTOML (builtins.readFile ../herdr-plugin.toml);

  # Package contract (the flake provides it): share/herdr/plugins/<id>.
  pluginDir = "${cfg.package}/share/herdr/plugins/${manifest.id}";

  renderValue =
    v:
    if lib.isBool v then
      (if v then "true" else "false")
    else if lib.isInt v then
      toString v
    else
      # TOML basic string: escape backslashes and quotes.
      "\"${lib.replaceStrings [ "\\" "\"" ] [ "\\\\" "\\\"" ] v}\"";
in
{
  options.programs.herdrPreviewGrid = {
    enable = lib.mkEnableOption "the herdr preview-grid plugin (store-path linked)";

    package = lib.mkOption {
      type = lib.types.package;
      default = config._module.args.herdrPreviewGridPackage or (throw "herdrPreviewGrid: set programs.herdrPreviewGrid.package — the module was imported without the flake's _module.args injection");
      defaultText = lib.literalExpression "the flake's own package";
      description = "Plugin package; must expose share/herdr/plugins/${manifest.id}.";
    };

    autoLink = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Run `herdr plugin link <package> --enabled` at activation. Disable to manage the registry yourself (e.g. keep a dev checkout linked).";
    };

    addToPath = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Prepend the package's bin/ to home.sessionPath so `herdr-preview-grid` resolves by name. Needed for keybindings that run the binary directly (e.g. the sidebar jump popup): store paths change on every rebuild, so hardcoding one would rot.";
    };

    settings = lib.mkOption {
      type = lib.types.attrsOf (lib.types.oneOf [ lib.types.str lib.types.int lib.types.bool ]);
      default = { };
      example = {
        charset = "jklfds";
        sort = "priority";
        master_width = 0;
      };
      description = "Plugin config.toml entries, written to ~/.config/herdr/plugins/config/${manifest.id}/config.toml. See config.example.toml for the full set (charset, master_width, preview, refresh_ms, sort, squeeze_blank_lines). Empty = file untouched.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.package != null;
        message = "herdrPreviewGrid: package must be set.";
      }
    ];

    home.sessionPath = lib.mkIf cfg.addToPath (lib.mkBefore [ "${cfg.package}/bin" ]);

    home.activation.herdrPreviewGridLink = lib.mkIf cfg.autoLink (
      lib.hm.dag.entryAfter [ "linkGeneration" ] ''
        herdr_bin=$(command -v herdr || true)
        if [ -n "$herdr_bin" ]; then
          "$herdr_bin" plugin link ${lib.escapeShellArg pluginDir} --enabled >/dev/null \
            || echo "herdr-preview-grid: plugin link failed (check 'herdr plugin list')" >&2
        else
          echo "herdr-preview-grid: herdr not on PATH — plugin not linked" >&2
        fi
      ''
    );

    home.file.".config/herdr/plugins/config/${manifest.id}/config.toml" = lib.mkIf (cfg.settings != { }) {
      # lib.sort (not lib.sortAttrs) — HM's lib on older releases lacks the latter.
      text =
        lib.concatMapStringsSep "\n" (name: "${name} = ${renderValue cfg.settings.${name}}")
          (lib.sort (a: b: a < b) (builtins.attrNames cfg.settings))
        + "\n";
    };
  };
}