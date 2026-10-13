#!/usr/bin/env python3
"""make mutate — мутационный прогон по scripts/tools/mutants.txt (формат — шапка файла).

Использование: make mutate [A="<id> …" | A=--check]

Прогон идёт на чистом дереве. Сначала базовый прогон тестов: красный — отказ (мутации без смысла), а кеш
go test после него держит результаты пакетов, которых мутация не касается. Каждая мутация: правка по
тексту кода (якорь «было» — ровно одно место файла, пробелы не важны) → gofumpt; правка SQL — sqlc
generate, правка HTTP-слоя — openapi.json (дрейф сгенерированного мутацию не убивает) → go build ./...
→ юнит-тесты (go test ./...) → юнит + integration (go test -tags=integration ./..., нужен Docker) →
откат к HEAD. Без -race: мутации — про логику, гонки держит гейт.

Итог — убитые по стадиям и выжившие: известные эквивалентные (строка «=» в спецификации) и новые; ещё —
мутации с устаревшим якорем и несобравшиеся (их правят в спецификации). Код 1 — есть новые выжившие или
таймауты. Результаты — .cache/mutate/<HEAD>.tsv: оборванный прогон на том же коммите продолжается с места,
правка, оставленная обрывом, откатывается. A="<id> …" — эти мутации заново, A=--check — только якоря.
"""
import fcntl
import os
import pathlib
import re
import subprocess
import sys
import time

ROOT = pathlib.Path(
    subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True).stdout.strip()
)
SPEC = ROOT / "scripts" / "tools" / "mutants.txt"
CACHE = ROOT / ".cache" / "mutate"
INFLIGHT = CACHE / "inflight"
# Правка этих путей меняет HTTP-контракт: openapi.json регенерируется, как сделал бы make gen.
HTTP_PATHS = ("/delivery/http/", "internal/platform/authgate/", "cmd/api/", "pkg/")
STAGE_TIMEOUT = 1800
# Окружение make и оболочки вызывающего до go и тестов не доходит — как у make check.
ENV = {
    k: v
    for k, v in os.environ.items()
    if not k.startswith(("MAKE", "BASH_FUNC_"))
    and k not in ("MFLAGS", "GNUMAKEFLAGS", "BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS", "API_TEST_MAIN_ARGS")
}


def fail(msg):
    print(f"FAIL: {msg}")
    sys.exit(1)


def git(*args):
    return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, check=True, env=ENV).stdout


def run(cmd):
    """Код выхода и вывод команды; None — таймаут."""
    try:
        r = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True, errors="replace", timeout=STAGE_TIMEOUT, env=ENV)
        return r.returncode, r.stdout + r.stderr
    except subprocess.TimeoutExpired as e:
        out = e.stdout.decode(errors="replace") if isinstance(e.stdout, bytes) else (e.stdout or "")
        return None, out + "\nTIMEOUT"


def parse(text):
    """Мутации спецификации: id, слой, что ломает, причина эквивалентности, правки (файл, было, стало)."""
    mutants, cur, edit = [], None, None
    for n, line in enumerate(text.splitlines(), 1):
        if line.startswith("## "):
            mid, _, rest = line[3:].partition(" ")
            layer, _, note = rest.partition(" — ")
            cur, edit = {"id": mid, "layer": layer, "note": note, "eq": "", "edits": []}, None
            mutants.append(cur)
        elif not line.strip() or line.startswith("#"):
            continue
        elif cur is None:
            fail(f"mutants.txt:{n}: строка вне блока мутации")
        elif line.startswith("= "):
            cur["eq"] = line[2:]
        elif line[:1] in "-+" and line[1:2] in ("", " "):
            if edit is None:
                fail(f"mutants.txt:{n}: строка «{line[0]}» до имени файла")
            edit["before" if line[0] == "-" else "after"].append(line[2:])
        else:
            edit = {"path": line.strip(), "before": [], "after": []}
            cur["edits"].append(edit)
    seen = set()
    for m in mutants:
        if m["id"] in seen:
            fail(f"mutants.txt: мутация {m['id']} — дважды")
        seen.add(m["id"])
        if not m["edits"] or any(not e["before"] for e in m["edits"]):
            fail(f"mutants.txt: у мутации {m['id']} нет правки или строк «было»")
    return mutants


def anchor(lines):
    return re.compile(r"\s+".join(re.escape(chunk) for chunk in " ".join(lines).split()))


def stale(m):
    """Причина, по которой мутация не применится к дереву, или пустая строка."""
    for e in m["edits"]:
        path = ROOT / e["path"]
        if not path.is_file():
            return f"нет файла {e['path']}"
        found = len(anchor(e["before"]).findall(path.read_text()))
        if found != 1:
            return f"«было» в {e['path']}: вхождений {found}"
    return ""


def apply(m):
    for e in m["edits"]:
        path = ROOT / e["path"]
        text = path.read_text()
        found = list(anchor(e["before"]).finditer(text))
        if len(found) != 1:
            return f"«было» в {e['path']} после предыдущей правки: вхождений {len(found)}"
        after = "\n".join(e["after"]).lstrip()
        path.write_text(text[: found[0].start()] + after + text[found[0].end():])
    return ""


def dirty():
    return [line for line in git("status", "--porcelain", "--untracked-files=all").decode().splitlines() if line.strip()]


def restore():
    """Дерево — к HEAD: до прогона оно было чистым, значит всё изменённое — правка мутации."""
    for path in filter(None, git("diff", "--name-only", "-z", "HEAD").decode().split("\0")):
        (ROOT / path).write_bytes(git("show", f"HEAD:{path}"))
    for path in filter(None, git("ls-files", "--others", "--exclude-standard", "-z").decode().split("\0")):
        (ROOT / path).unlink()
    left = dirty()
    if left:
        fail("дерево не вернулось к HEAD — проверь руками: " + "; ".join(left))


def failed(out):
    tests = re.findall(r"^\s*--- FAIL: (\S+)", out, re.M)
    top = [t for t in tests if "/" not in t] or tests
    pkgs = re.findall(r"^FAIL\s+(\S+)", out, re.M)
    return ", ".join(list(dict.fromkeys(top))[:6]) or ", ".join(list(dict.fromkeys(pkgs))[:4]) or out.strip()[-200:]


def mutate(m, log):
    """Вердикт мутации: unit, integration (убита), survived, invalid, timeout — и подробность."""
    reason = stale(m) or apply(m)
    if reason:
        return "stale", reason
    paths = [e["path"] for e in m["edits"]]
    steps = []
    gofiles = [p for p in paths if p.endswith(".go")]
    if gofiles:
        steps.append(("gofumpt", ["gofumpt", "-extra", "-w", *gofiles]))
    if any(p.endswith(".sql") for p in paths):
        steps.append(("sqlc", ["sqlc", "generate"]))
    if any(any(h in p for h in HTTP_PATHS) for p in paths):
        steps.append(("openapi", ["go", "test", "./cmd/api", "-run", "^TestOpenAPISpec$", "-count=1", "-args", "-update"]))
    steps.append(("build", ["go", "build", "./..."]))
    for name, cmd in steps:
        code, out = run(cmd)
        log.append(f"$ {' '.join(cmd)}  (exit {code})\n{out}")
        if code != 0:
            return "invalid", f"{name}: {out.strip()[-200:]}"
    log.append("$ git diff\n" + git("diff").decode())
    for stage, cmd in (("unit", ["go", "test", "./..."]), ("integration", ["go", "test", "-tags=integration", "./..."])):
        t0 = time.time()
        code, out = run(cmd)
        log.append(f"$ {' '.join(cmd)}  ({int(time.time() - t0)}s, exit {code})\n{out}")
        if code is None:
            return "timeout", stage
        if code != 0:
            return stage, failed(out)
    return "survived", ""


def main(args):
    mutants = parse(SPEC.read_text())
    if args == ["--check"]:
        bad = [(m["id"], stale(m)) for m in mutants]
        bad = [(mid, why) for mid, why in bad if why]
        for mid, why in bad:
            print(f"  {mid}: {why}")
        if bad:
            fail(f"мутаций с устаревшим якорем: {len(bad)} из {len(mutants)} — перепиши их блоки в {SPEC.relative_to(ROOT)}")
        print(f"OK: якоря {len(mutants)} мутаций — по одному месту в дереве")
        return 0
    by_id = {m["id"]: m for m in mutants}
    unknown = [a for a in args if a not in by_id]
    if unknown:
        fail("нет таких мутаций: " + " ".join(unknown))

    CACHE.mkdir(parents=True, exist_ok=True)
    lock = open(CACHE / "lock", "w")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except OSError:
        fail("make mutate уже идёт в этом дереве")
    if INFLIGHT.exists():
        print(f"→ откат правки оборванной мутации {INFLIGHT.read_text().strip()}")
        restore()
        INFLIGHT.unlink()
    elif dirty():
        fail("дерево не чистое — make mutate правит файлы и откатывает их к HEAD: закоммить или отложи правки")
    if subprocess.run(["go", "env", "GOFLAGS"], capture_output=True, text=True, env=ENV).stdout.strip():
        fail("make mutate не запускается с GOFLAGS — тесты шли бы не так, как в гейте")

    head = git("rev-parse", "HEAD").decode().strip()
    results = CACHE / f"{head}.tsv"
    for old in CACHE.glob("*.tsv"):
        if old != results:
            old.unlink()
    logs = CACHE / "logs"
    logs.mkdir(exist_ok=True)
    done = {}
    if results.exists():
        for line in results.read_text().splitlines():
            mid, verdict, _, detail = (line.split("\t") + ["", "", "", ""])[:4]
            done[mid] = (verdict, detail)
    selected = [by_id[a] for a in args] if args else mutants
    todo = [m for m in selected if args or m["id"] not in done]

    if todo:
        print(f"→ базовый прогон: go test и go test -tags=integration по всему дереву ({len(todo)} мутаций впереди)")
        for cmd in (["go", "test", "./..."], ["go", "test", "-tags=integration", "./..."]):
            code, out = run(cmd)
            if code != 0:
                print(out[-3000:])
                fail(f"базовый прогон {' '.join(cmd)} красный — мутации на красных тестах без смысла")
    for k, m in enumerate(todo, 1):
        t0 = time.time()
        INFLIGHT.write_text(m["id"])
        log = []
        try:
            verdict, detail = mutate(m, log)
        finally:
            restore()
            INFLIGHT.unlink()
        dt = int(time.time() - t0)
        (logs / f"{m['id']}.log").write_text("\n\n".join(log))
        done[m["id"]] = (verdict, detail)
        with results.open("a") as f:
            f.write("\t".join([m["id"], verdict, f"{dt}s", detail.replace("\t", " ").replace("\n", " ")]) + "\n")
        print(f"[{k}/{len(todo)}] {m['id']} {m['layer']} — {verdict} {dt}s {detail[:120]}".rstrip(), flush=True)
    return summary(selected, done, head)


def summary(selected, done, head):
    groups = {}
    for m in selected:
        verdict = done.get(m["id"], ("", ""))[0]
        if verdict == "survived":
            verdict = "equivalent" if m["eq"] else "new"
        elif verdict in ("unit", "integration") and m["eq"]:
            verdict = "equivalent-killed"
        groups.setdefault(verdict, []).append(m)
    unit, integration, equivalent = (groups.get(k, []) for k in ("unit", "integration", "equivalent"))
    print(f"\nИтог (HEAD {head[:7]}, мутаций {len(selected)}): убиты {len(unit) + len(integration)} — юнит {len(unit)}, "
          f"integration {len(integration)}")
    if equivalent:
        print(f"выжили известные эквивалентные ({len(equivalent)}): " + " ".join(m["id"] for m in equivalent))
    for key, title in (
        ("new", "ВЫЖИЛИ — НОВЫЕ: тесты не видят поломку — добавь тест или, если правка ненаблюдаема, строку «=» "
                "(дифф и вывод тестов — .cache/mutate/logs/<id>.log)"),
        ("timeout", "таймаут — перегони: make mutate A=\"<id>\""),
        ("stale", "устаревший якорь — перепиши блок под код"),
        ("invalid", "не собирается — поправь правку «стало»"),
        ("equivalent-killed", "эквивалентная убита — проверь строку «=»"),
    ):
        if groups.get(key):
            print(f"{title} ({len(groups[key])}):")
            for m in groups[key]:
                detail = done[m["id"]][1]
                print(f"  {m['id']} {m['layer']} — {m['note']}" + (f" [{detail}]" if detail else ""))
    bad = len(groups.get("new", [])) + len(groups.get("timeout", []))
    if bad:
        fail(f"новых выживших — {len(groups.get('new', []))}, таймаутов — {len(groups.get('timeout', []))}")
    print("OK: новых выживших нет")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
