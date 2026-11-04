"""Stage docs for Pages without changing Markdown as viewed on GitHub."""
import json
import os
from pathlib import Path
import re
import shutil
from urllib.parse import quote

root = Path(__file__).resolve().parents[2]
source = root / "docs"
target = root / "_pages_source"
repository = os.environ.get("GITHUB_REPOSITORY", "darianJmy/guanlan-ops")
branch = os.environ.get("REPOSITORY_BRANCH", "main")
repository_url = f"https://github.com/{repository}"
if target.exists():
    shutil.rmtree(target)
shutil.copytree(source, target)

for original in source.rglob("*.md"):
    content = original.read_text()
    # Source-code links outside docs should open GitHub, not a missing Pages URL.
    def source_link(match):
        relative = match.group(1)
        path = (original.parent / relative).resolve()
        if path.is_relative_to(source):
            return match.group(0)
        if not path.is_relative_to(root) or not path.is_file():
            raise ValueError(f"Broken repository link in {original}: {relative}")
        return f"]({repository_url}/blob/{quote(branch, safe='')}/{path.relative_to(root).as_posix()})"
    content = re.sub(r"\]\((\.\./[^\s)#]+)\)", source_link, content)
    if not content.startswith("---\n"):
        heading = re.search(r"^# (.+)$", content, re.MULTILINE)
        title = heading.group(1) if heading else original.stem
        content = "---\ntitle: " + json.dumps(title, ensure_ascii=False) + "\n---\n\n" + content
    (target / original.relative_to(source)).write_text(content)

configured = os.environ.get("PAGES_CONFIGURED") == "true"
settings = {
    "url": os.environ.get("PAGES_ORIGIN") or f"https://{repository.split('/')[0].lower()}.github.io",
    "baseurl": os.environ.get("PAGES_BASE_PATH", "") if configured else "/" + repository.split("/")[1],
    "repository": repository,
    "repository_url": repository_url,
    "repository_branch": branch,
}
with (target / "_config.yml").open("a") as config:
    for key, value in settings.items():
        config.write(f"\n{key}: {json.dumps(value)}\n")
print(f"Prepared {len(list(source.rglob('*.md')))} documentation pages for {repository}")
