#!/usr/bin/env python3
"""make mutate — мутационный прогон по scripts/tools/mutants.txt (формат — шапка файла).

Использование: make mutate [A="[--strict] <id> …" | A="--check [<id> …]"]

Мутации идут по HEAD в своём worktree .cache/mutate/wt: дерево разработчика прогон не трогает, незакоммиченное в
прогон не входит. Сначала базовый прогон тестов: красный — отказ (мутации без смысла), а кеш go test после него
держит результаты пакетов, которых мутация не касается. Каждая мутация: правка по тексту кода (якорь «было» —
ровно одно место файла, пробелы не важны) → gofumpt; правка SQL — sqlc generate, правка HTTP-слоя — openapi.json
(дрейф сгенерированного мутацию не убивает) → go build ./... → юнит-тесты (go test ./...) → юнит + integration
(go test -tags=integration ./..., нужен Docker) → worktree — снова HEAD. Без -race: мутации — про логику, гонки
держит гейт. Временные файлы go каждой стадии — в .cache/mutate/tmp: стадию, снятую по таймауту или обрывом, они не
переживают.

Спецификация растёт с кодом: файл кода (Go-файл internal/, cmd/ и pkg/ с функцией — кроме тестов, сгенерированного
и internal/testkit/ — и запросы sqlc) без единой мутации в отчёте назван — новой логике нужны свои блоки.

Итог — убитые по стадиям и выжившие: известные эквивалентные (строка «=» в спецификации) и новые; ещё — мутации с
устаревшим якорем и несобравшиеся (их правят в спецификации) и файлы кода без мутаций. Код 1 — есть новые выжившие или
таймауты; с --strict — ещё устаревшие якоря, несобравшиеся правки, эквивалентные, которые тесты убили (строка «=»
неверна), и файлы кода без мутаций: спецификация, разошедшаяся с кодом, меряет меньше (шаг релиза). Результаты —
.cache/mutate/<HEAD>.tsv: оборванный прогон на том же коммите продолжается с места. A="<id> …" — эти мутации заново,
A=--check — только якоря спецификации рабочего дерева (с id — этих мутаций) и файлы кода без мутаций.
"""
import contextlib
import fcntl
import os
import pathlib
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time

ROOT = pathlib.Path(
    subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True).stdout.strip()
)
SPEC = pathlib.Path("scripts", "tools", "mutants.txt")
CACHE = ROOT / ".cache" / "mutate"
WT = CACHE / "wt"
TMP = CACHE / "tmp"
# Правка этих путей меняет HTTP-контракт: openapi.json регенерируется, как сделал бы make gen.
HTTP_PATHS = ("/delivery/http/", "internal/platform/authgate/", "cmd/api/", "pkg/")
STAGE_TIMEOUT = 1800
# Окружение оболочки вызывающего снимает Makefile с рецепта make mutate, как у make check; перезапуск тестов cmd/api
# в main (API_TEST_MAIN_ARGS) make check отвергает — здесь он снят.
ENV = {k: v for k, v in os.environ.items() if k != "API_TEST_MAIN_ARGS"}


def fail(msg):
    print(f"FAIL: {msg}")
    sys.exit(1)


def git(*args, cwd=ROOT):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, check=True, env=ENV).stdout.decode()


def kill(p):
    """Группа процессов команды — SIGKILL; группы, которая уже вышла, нет — и снимать нечего."""
    with contextlib.suppress(ProcessLookupError):
        os.killpg(p.pid, signal.SIGKILL)


def run(cmd):
    """Код выхода и вывод команды в worktree раннера; None — таймаут. Команда — своя группа процессов: таймаут и
    обрыв прогона снимают её целиком, с тестовыми бинарями go test, а не один go; её временные файлы go (GOTMPDIR —
    сборка и бинари тестов) уходят вместе с ней."""
    TMP.mkdir(parents=True, exist_ok=True)
    tmp = tempfile.mkdtemp(dir=TMP)
    p = subprocess.Popen(cmd, cwd=WT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, errors="replace",
                         env=dict(ENV, GOTMPDIR=tmp), start_new_session=True)
    try:
        out = p.communicate(timeout=STAGE_TIMEOUT)[0]
        return p.returncode, out
    except subprocess.TimeoutExpired:
        kill(p)
        return None, p.communicate()[0] + "\nTIMEOUT"
    except BaseException:
        kill(p)
        p.wait()
        raise
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


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


def stale(m, tree):
    """Причина, по которой мутация не применится к дереву tree, или пустая строка."""
    for e in m["edits"]:
        path = tree / e["path"]
        if not path.is_file():
            return f"нет файла {e['path']}"
        found = len(anchor(e["before"]).findall(path.read_text()))
        if found != 1:
            return f"«было» в {e['path']}: вхождений {found}"
    return ""


GENERATED = re.compile(r"^// Code generated .* DO NOT EDIT\.$", re.M)
QUERIES = re.compile(r"internal/domain/[^/]+/infra/postgres/queries/[^/]+\.sql")


def untested(mutants, tree, files):
    """Файлы кода дерева tree (пути files) без единой мутации: Go-файлы internal/, cmd/ и pkg/ с функцией, кроме
    тестов, сгенерированного и internal/testkit/, и запросы sqlc."""
    covered = {e["path"] for m in mutants for e in m["edits"]}
    code = []
    for f in files:
        if f in covered or not (tree / f).is_file():
            continue
        if QUERIES.fullmatch(f):
            code.append(f)
        elif f.endswith(".go") and not f.endswith("_test.go") and f.startswith(("internal/", "cmd/", "pkg/")) \
                and not f.startswith("internal/testkit/"):
            text = (tree / f).read_text()
            if re.search(r"^func ", text, re.M) and not GENERATED.search(text):
                code.append(f)
    return code


def apply(m):
    for e in m["edits"]:
        path = WT / e["path"]
        text = path.read_text()
        found = list(anchor(e["before"]).finditer(text))
        if len(found) != 1:
            return f"«было» в {e['path']} после предыдущей правки: вхождений {len(found)}"
        after = "\n".join(e["after"]).lstrip()
        path.write_text(text[: found[0].start()] + after + text[found[0].end():])
    return ""


def reset(head):
    """Worktree раннера — ровно HEAD разработчика: правка прошлой мутации, и оборванной, уходит."""
    if not (WT / ".git").exists():
        git("worktree", "prune")
        git("worktree", "add", "-q", "--detach", str(WT), head)
    git("checkout", "-q", "-f", "--detach", head, cwd=WT)
    git("clean", "-q", "-fd", cwd=WT)


def failed(out):
    tests = re.findall(r"^\s*--- FAIL: (\S+)", out, re.M)
    top = [t for t in tests if "/" not in t] or tests
    pkgs = re.findall(r"^FAIL\s+(\S+)", out, re.M)
    return ", ".join(list(dict.fromkeys(top))[:6]) or ", ".join(list(dict.fromkeys(pkgs))[:4]) or out.strip()[-200:]


def mutate(m, log):
    """Вердикт мутации: unit, integration (убита), survived, invalid, timeout — и подробность."""
    reason = stale(m, WT) or apply(m)
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
    log.append("$ git diff\n" + git("diff", cwd=WT))
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
    strict = "--strict" in args
    args = [a for a in args if a != "--strict"]
    if args[:1] == ["--check"]:
        return check(args[1:], strict)

    CACHE.mkdir(parents=True, exist_ok=True)
    lock = open(CACHE / "lock", "w")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except OSError:
        fail("make mutate уже идёт в этом дереве")
    if subprocess.run(["go", "env", "GOFLAGS"], capture_output=True, text=True, env=ENV).stdout.strip():
        fail("make mutate не запускается с GOFLAGS — тесты шли бы не так, как в гейте")
    # SIGTERM и SIGHUP (kill, закрытый терминал) — как Ctrl-C: прогон выходит через откат worktree.
    for sig in (signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, lambda signum, _: sys.exit(128 + signum))

    head = git("rev-parse", "HEAD").strip()
    if git("status", "--porcelain", "--untracked-files=all").strip():
        print(f"→ незакоммиченное в прогон не входит: мутации идут по HEAD {head[:7]}")
    reset(head)
    mutants = parse((WT / SPEC).read_text())
    bare = [] if args else untested(mutants, WT, git("ls-files", cwd=WT).splitlines())
    by_id = {m["id"]: m for m in mutants}
    unknown = [a for a in args if a not in by_id]
    if unknown:
        fail("нет таких мутаций: " + " ".join(unknown))

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
        print(f"→ базовый прогон: go test и go test -tags=integration по HEAD {head[:7]} ({len(todo)} мутаций впереди)")
        for cmd in (["go", "test", "./..."], ["go", "test", "-tags=integration", "./..."]):
            code, out = run(cmd)
            if code != 0:
                print(out[-3000:])
                fail(f"базовый прогон {' '.join(cmd)} красный — мутации на красных тестах без смысла")
    for k, m in enumerate(todo, 1):
        t0 = time.time()
        log = []
        try:
            verdict, detail = mutate(m, log)
        finally:
            reset(head)
        dt = int(time.time() - t0)
        detail = " ".join(detail.split())
        (logs / f"{m['id']}.log").write_text("\n\n".join(log))
        done[m["id"]] = (verdict, detail)
        with results.open("a") as f:
            f.write("\t".join([m["id"], verdict, f"{dt}s", detail]) + "\n")
        print(f"[{k}/{len(todo)}] {m['id']} {m['layer']} — {verdict} {dt}s {detail[:120]}".rstrip(), flush=True)
    return summary(selected, done, head, strict, bare)


def check(ids, strict):
    """A=--check: якоря спецификации рабочего дерева (с ids — этих мутаций) и файлы кода без мутаций."""
    mutants = parse((ROOT / SPEC).read_text())
    by_id = {m["id"]: m for m in mutants}
    unknown = [a for a in ids if a not in by_id]
    if unknown:
        fail("нет таких мутаций: " + " ".join(unknown))
    selected = [by_id[a] for a in ids] if ids else mutants
    bad = [(m["id"], stale(m, ROOT)) for m in selected]
    bad = [(mid, why) for mid, why in bad if why]
    for mid, why in bad:
        print(f"  {mid}: {why}")
    if bad:
        fail(f"мутаций с устаревшим якорем: {len(bad)} из {len(selected)} — перепиши их блоки в {SPEC}")
    bare = [] if ids else untested(mutants, ROOT, git("ls-files", "-co", "--exclude-standard").splitlines())
    for f in bare:
        print(f"  {f}: нет мутаций")
    if bare and strict:
        fail(f"файлов кода без мутаций: {len(bare)} — блоки в {SPEC} (rule tests-pipeline)")
    print(f"OK: якоря {len(selected)} мутаций — по одному месту в дереве"
          + (f"; файлов кода без мутаций: {len(bare)} — блоки в {SPEC}, с --strict — отказ" if bare else ""))
    return 0


def summary(selected, done, head, strict, bare):
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
    for key, title, hint in (
        ("new", "ВЫЖИЛИ — НОВЫЕ", "тесты не видят поломку — добавь тест или, если правка ненаблюдаема, строку «=» "
                                  "(дифф и вывод тестов — .cache/mutate/logs/<id>.log)"),
        ("timeout", "таймаут", "перегони: make mutate A=\"<id>\""),
        ("stale", "устаревший якорь", "перепиши блок под код"),
        ("invalid", "не собирается", "поправь правку «стало»"),
        ("equivalent-killed", "эквивалентная убита", "тест её видит — сними строку «=»"),
    ):
        if groups.get(key):
            print(f"{title} ({len(groups[key])}): {hint}")
            for m in groups[key]:
                detail = done[m["id"]][1]
                print(f"  {m['id']} {m['layer']} — {m['note']}" + (f" [{detail}]" if detail else ""))
    if bare:
        print(f"файлы кода без мутаций ({len(bare)}): новой логике — свои блоки в {SPEC} (rule tests-pipeline)")
        for f in bare:
            print(f"  {f}")
    counts = {key: len(groups.get(key, [])) for key in ("new", "timeout", "stale", "invalid", "equivalent-killed")}
    loose = counts["stale"] + counts["invalid"] + counts["equivalent-killed"] + len(bare)
    if counts["new"] + counts["timeout"] + (loose if strict else 0):
        fail(f"новых выживших — {counts['new']}, таймаутов — {counts['timeout']}"
             + (f", устаревших якорей — {counts['stale']}, несобравшихся — {counts['invalid']}, эквивалентных убитых — "
                f"{counts['equivalent-killed']}, файлов кода без мутаций — {len(bare)} (--strict)" if strict else ""))
    print("OK: новых выживших нет" + (", спецификация совпадает с кодом" if strict else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
