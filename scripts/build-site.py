"""Publish exactly the embedded app, with a synthetic browser workspace."""
from pathlib import Path
import argparse

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("--check", action="store_true")
args = parser.parse_args()
for name in ("index.html", "claude.html", "claude.js", "claude.css", "dashboard.js", "dashboard.css", "favicon.svg", "i18n.js", "i18n-ko.js", "inspector.js"):
    data = (root / "internal" / "webui" / name).read_text()
    if name == "index.html":
        data = data.replace("{{.}}", '{"mode":"demo"}')
    if name == "claude.html":
        data = data.replace("{{.}}", '{"mode":"native-demo"}')
    destination = root / "site" / ("experiments.html" if name == "index.html" else "index.html" if name == "claude.html" else name)
    if args.check:
        if not destination.exists() or destination.read_text() != data:
            raise SystemExit(f"Stale site asset: {name}. Run python3 scripts/build-site.py")
    else:
        destination.write_text(data)
print("Site assets match the embedded dashboard." if args.check else "Built public dashboard.")
