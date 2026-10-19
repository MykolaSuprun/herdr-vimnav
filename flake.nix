{
  description = "herdr-preview-grid — master-detail switcher for herdr: Nix package + Home Manager module";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager = {
      url = "github:nix-community/home-manager";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      home-manager,
      ...
    }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
      # Single source of truth for the version: the plugin manifest.
      manifest = builtins.fromTOML (builtins.readFile ./herdr-plugin.toml);
    in
    {
      packages = forAll (pkgs: rec {
        herdr-preview-grid = pkgs.buildGoModule {
          pname = "herdr-preview-grid";
          inherit (manifest) version;
          src = pkgs.lib.cleanSourceWith {
            src = ./.;
            filter =
              path: type:
              let
                base = baseNameOf (toString path);
              in
              !(type == "directory" && base == ".git");
          };
          vendorHash = "sha256-EDsq9RzYhe6I2Knr/fL8miUUo4ixsilwgY248QxKRfc=";
          subPackages = [ "cmd/herdr-preview-grid" ];
          # Static output, matching the manifest [[build]] artifact: buildGoModule
          # reads env.CGO_ENABLED (nixpkgs' go defaults it to 1).
          env.CGO_ENABLED = "0";

          # Plugin store layout — serves BOTH consumption paths:
          #   - `herdr plugin link <this store path>` (manifest at the root
          #     so the manifest's ./bin/herdr-preview-grid resolves; link
          #     never runs [[build]]);
          #   - generic Nix modules linking the convention path
          #     share/herdr/plugins/<id> (wimpysworld-style).
          postInstall = ''
            # buildGoModule already installed $out/bin/herdr-preview-grid.
            test -x $out/bin/herdr-preview-grid
            install -Dm644 herdr-plugin.toml $out/herdr-plugin.toml
            install -Dm644 README.md $out/share/doc/herdr-preview-grid/README.md
            install -Dm644 config.example.toml $out/share/doc/herdr-preview-grid/config.example.toml
            mkdir -p $out/share/herdr/plugins
            # THREE ups from share/herdr/plugins/<id> back to the package
            # root — four would overshoot to /nix/store (live-caught by the
            # container E2E when herdr plugin link hit ENOENT). Pinned by
            # checks.layout below via readlink -f equality.
            ln -s ../../.. $out/share/herdr/plugins/${manifest.id}
          '';

          # Nix-aware plugin contract: lets `programs.herdr-hpm` entries be a
          # bare package (the id is read, never retyped). Kept honest by the
          # manifest-id check below.
          passthru.herdrPluginId = manifest.id;

          meta = {
            description = "herdr plugin: master-detail switcher for spaces/agents (live pane previews, MRU priority sort, sidebar jump codes)";
            homepage = "https://gitlab.com/mykolas-open/herdr-vimnav";
            license = pkgs.lib.licenses.mit;
            # The manifest's platforms = ["linux", "macos"] is herdr shorthand,
            # not Nix platform strings; the flake targets these systems.
            platforms = pkgs.lib.platforms.unix ++ pkgs.lib.platforms.darwin;
            mainProgram = "herdr-preview-grid";
          };
        };
        default = herdr-preview-grid;
      });

      # buildGoModule's checkPhase runs `go test ./...` — the package check is
      # therefore also the Go test suite.
      checks = forAll (pkgs: {
        package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;

        # passthru.herdrPluginId and the manifest must not drift apart: the id
        # is what plugin managers link/prune on, so a mismatch is a footgun.
        manifest-id =
          let
            pkg = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
            declared = pkg.passthru.herdrPluginId;
            fromManifest = (builtins.fromTOML (builtins.readFile ./herdr-plugin.toml)).id;
          in
          if declared == fromManifest then
            pkgs.runCommand "herdr-preview-grid-manifest-id" { } ''
              echo "plugin id: ${declared}" > $out
            ''
          else
            throw "passthru.herdrPluginId (${declared}) != herdr-plugin.toml id (${fromManifest})";

        # Store-layout contract: the share/herdr/plugins/<id> symlink must
        # resolve back to the package root. readlink -f equality — the exact
        # invariant `herdr plugin link` needs (a wrong up-count overshoots and
        # link dies with ENOENT; live-caught by the container E2E).
        layout = pkgs.runCommand "herdr-preview-grid-layout" { } ''
          pkg=${self.packages.${pkgs.stdenv.hostPlatform.system}.default}
          root=$(readlink -f "$pkg/share/herdr/plugins/preview-grid")
          if [ "$root" != "$pkg" ]; then
            echo "layout broken: share/herdr/plugins/preview-grid resolves to $root, not $pkg" >&2
            exit 1
          fi
          test -f "$root/herdr-plugin.toml"
          test -x "$root/bin/herdr-preview-grid"
          touch $out
        '';

        # Real Home Manager evaluation of the module (options merge, package
        # injection, TOML rendering) without a full home-manager build.
        module-eval = pkgs.runCommand "herdr-preview-grid-hm-module-eval" { } ''
          cat > eval.json <<'JSON'
          ${builtins.toJSON (
            let
              hm = home-manager.lib.homeManagerConfiguration {
                inherit pkgs;
                modules = [
                  self.homeManagerModules.default
                  {
                    home.stateVersion = "25.05";
                    home.username = "hpg-check";
                    home.homeDirectory = "/home/hpg-check";
                    programs.herdrPreviewGrid.enable = true;
                    programs.herdrPreviewGrid.settings = {
                      charset = "jklfds";
                      master_width = 0;
                      preview = true;
                    };
                  }
                ];
              };
              cfg = hm.config.programs.herdrPreviewGrid;
            in
            {
              inherit (cfg) enable autoLink settings;
              packageName = cfg.package.name;
              hasLinkActivation = hm.config.home.activation ? herdrPreviewGridLink;
              linkAfter = hm.config.home.activation.herdrPreviewGridLink.after;
              linkData = hm.config.home.activation.herdrPreviewGridLink.data or null;
              sessionPathHasBinary = builtins.any (
                p: nixpkgs.lib.hasInfix "-herdr-preview-grid-" p && nixpkgs.lib.hasSuffix "/bin" p
              ) hm.config.home.sessionPath;
              settingsFile =
                hm.config.home.file.".config/herdr/plugins/config/preview-grid/config.toml".text or null;
            }
          )}
          JSON
          # toJSON is compact (no whitespace after ':') — match that.
          grep -q '"enable":true' eval.json
          grep -q '"autoLink":true' eval.json
          grep -q '"hasLinkActivation":true' eval.json
          grep -q '"sessionPathHasBinary":true' eval.json
          grep -q 'linkGeneration' eval.json
          grep -q 'plugin link' eval.json
          grep -q 'share/herdr/plugins/preview-grid' eval.json
          grep -q -- '--enabled' eval.json
          grep -q 'jklfds' eval.json
          grep -q 'master_width = 0' eval.json
          grep -q 'preview = true' eval.json
          test -s eval.json && echo "module eval ok" > $out
        '';
      });

      homeManagerModules.default =
        { pkgs, ... }:
        {
          imports = [ ./nix/home-manager-module.nix ];
          # Consumers get the flake's own package for free.
          _module.args.herdrPreviewGridPackage = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
        };

      # `nix fmt` — the repo's canonical style (also in the devShell).
      formatter = forAll (pkgs: pkgs.nixfmt-rfc-style);

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
          ];
        };
      });
    };
}
