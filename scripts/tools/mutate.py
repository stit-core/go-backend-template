#!/usr/bin/env python3
"""make mutate — мутационный прогон по scripts/tools/mutants.txt (формат — шапка файла).

Использование: make mutate [A="[--strict] [--fresh] <id> …" | A="--check [<id> …]" | A="--build [<id> …]" |
                            A="--anchors [<id> …]"]

Мутации идут по рабочему дереву разработчика в своём worktree .cache/mutate/wt: снимок отслеживаемых файлов (правки и
новые файлы в индексе, и git add -N; неотслеживаемые не входят) во временном индексе — спецификацию и код можно
проверить до коммита; само дерево и индекс разработчика прогон не трогает. Сначала базовый прогон тестов: красный — отказ (мутации без
смысла), а кеш go test после него держит результаты пакетов, которых мутация не касается. Каждая мутация: правка по тексту кода (якорь «было» —
ровно одно место файла, пробелы не важны) → gofumpt; правка SQL — sqlc generate, правка HTTP-слоя — openapi.json
(дрейф сгенерированного мутацию не убивает) → компиляция всех пакетов без линковки (go list -export ./...), программа,
чей файл задела правка, ещё и линкуется → юнит-тесты (go test) → юнит + integration (go test -tags=integration, нужен
Docker) → worktree — снова снимок. internal/archtest — последним, после integration: archtest читает всё дерево, и любая правка
снимает его кеш go test, а поломку логики видит редко — убитой раньше он не нужен; убитая им — юнит-стадия (тест
юнит-стадии), мутацию, которую убили бы и он, и integration, убивает integration. Выжившая проходит все тесты обеих
стадий, как у go test ./... стадий. archtest с тегом integration, у чьего теста и зависимостей те же файлы, что без
тега, — тот же бинарь: второй раз не гоняется. Без -race: мутации — про логику, гонки держит гейт. Временные файлы go каждой
стадии — в .cache/mutate/tmp: стадию, снятую по таймауту или обрывом, они не переживают, а оставленные убитым раннером
снимает следующий прогон.

Мутации берут из общей очереди исполнители — каждый в своём worktree (.cache/mutate/wt, wt-2, …); базовый прогон — в
первом, кеш go test остальных деревьев греют их первые мутации (кеш привязан к каталогу пакета); результат пишется в
results.tsv сразу, как готов, а строки хода — в порядке спецификации, как у одного исполнителя. Исполнителей по умолчанию — половина ядер слота (GOMAXPROCS,
без него — машины), не больше, чем помещается в доступную память по JOB_MEMORY_GB сверх запаса MEMORY_RESERVE_GB, и
не больше мутаций; явно — MUTATE_JOBS. Обрыв снимает команды всех исполнителей с их группами процессов, worktree — снова снимок; лишние worktree
прошлого прогона с большим числом исполнителей снимаются.

Серверы integration — один раз на прогон: модулю, чьи тесты зовут Start* кита golibinfra testkit с общим режимом
(SharedEnv), раннер поднимает Postgres и пулы NATS, Redis и MinIO — контейнеры образов кита с именем
mutate-<прогон>-<вид>-<номер> и метками mutate.run=<прогон> и mutate.repo=<репо>, — и путь их
файла отдаёт командам прогона в TESTKIT_SHARED: тест берёт свою базу или сервер в аренду (README кита § Общий режим).
Контейнеры прогона снимаются по метке в конце и при обрыве (SIGTERM, SIGHUP, Ctrl-C), оставленные убитым раннером —
следующим прогоном репо; без вызовов кита или кит без общего режима — контейнер на вызов Start*.

Спецификация растёт с кодом: файл кода (Go-файл internal/, cmd/ и pkg/ с функцией — кроме тестов, сгенерированного
и internal/testkit/ — и запросы sqlc) без единой мутации в отчёте назван — новой логике нужны свои блоки.

Итог — убитые по стадиям и выжившие: известные эквивалентные (строка «=» в спецификации) и новые; ещё — мутации с
устаревшим якорем и несобравшиеся (их правят в спецификации) и файлы кода без мутаций. Код 1 — есть новые выжившие или
таймауты; с --strict — ещё устаревшие якоря, несобравшиеся правки, эквивалентные, которые тесты убили (строка «=»
неверна), и файлы кода без мутаций: спецификация, разошедшаяся с кодом, меряет меньше (шаг релиза).

Результаты — .cache/mutate/results.tsv по ключу мутации: хеш её блока и файлов, которые она правит, в снимке, и дерево
снимка; у мутации в счёт — последняя строка. Убитая мутация с тем же ключом не гоняется заново, пока на месте тест,
который её убил: правка спецификации, другого файла или коммит без них прогон не повторяют, оборванный прогон
продолжается с места. Выжившие, несобравшиеся, устаревшие и таймауты идут заново всегда. Тест, ослабленный в другом
файле, такой прогон не видит — поэтому --strict (шаг релиза) берёт только вердикты прогонов того же снимка, кроме
несобравшихся и таймаутов: релиз гоняет мутации заново, а оборванный строгий прогон продолжается с места; A=--fresh
гонит все заново.
A="<id> …" — эти мутации заново, A=--build — каждая мутация (с id — эти) только собирается, без тестов и Docker:
несобирающиеся правки за минуты, а не за полный прогон; A=--check — якоря спецификации (с id — этих мутаций), файлы
кода без мутаций и та же сборка; A=--anchors — только якоря и файлы кода без мутаций, за секунды (их сверяет
make new-domain).
"""
import contextlib
import fcntl
import hashlib
import json
import os
import pathlib
import queue
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request

ROOT = pathlib.Path(
    subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True, check=True).stdout.strip()
)
SPEC = pathlib.Path("scripts", "tools", "mutants.txt")
CACHE = ROOT / ".cache" / "mutate"
WT = CACHE / "wt"
TMP = CACHE / "tmp"
# Правка этих путей меняет HTTP-контракт: openapi.json регенерируется, как сделал бы make gen.
HTTP_PATHS = ("/delivery/http/", "internal/platform/authgate/", "cmd/api/", "pkg/")
ARCHTEST = "internal/archtest"
# Тесты раннера (TestMutate*) гоняют этот файл на игрушечных модулях: правка кода сервиса их вердикт не меняет, а каждый
# — десятки прогонов раннера; стадии мутации их пропускают, их держит гейт.
RUNNER_TESTS = "^TestMutate"
# Файлы теста пакета и его зависимостей: с теми же файлами бинарь теста тот же.
TEST_FILES = ("{{.ImportPath}} {{.GoFiles}} {{.CgoFiles}} {{.CFiles}} {{.SFiles}} {{.TestGoFiles}} {{.XTestGoFiles}} "
              "{{.EmbedFiles}} {{.TestEmbedFiles}} {{.XTestEmbedFiles}}")
MAIN = re.compile(r"^package\s+main\b", re.M)
STAGE_TIMEOUT = 1800
# Окружение оболочки вызывающего снимает Makefile с рецепта make mutate, как у make check; перезапуск тестов cmd/api
# в main (API_TEST_MAIN_ARGS) make check отвергает — здесь он снят.
ENV = {k: v for k, v in os.environ.items() if k not in ("API_TEST_MAIN_ARGS", "TESTKIT_SHARED", "MUTATE_JOBS")}
# go.work разработчика в корне репо (кит с соседней ветки) go находит и из .cache/mutate/wt*, а модуля worktree в нём
# нет — команды go там падали бы; прогон идёт по go.mod снимка.
ENV["GOWORK"] = "off"
# Пик памяти исполнителя — сборка, линковка и бинари тестов одной мутации — с запасом, и запас под серверы integration
# и систему: исполнителей по умолчанию — не больше, чем помещается.
JOB_MEMORY_GB, MEMORY_RESERVE_GB = 1.2, 1.0
# Команды исполнителей — их группы процессов снимает обрыв прогона; STOP — обрыв: результат снятой команды не пишется.
RUNNING, RUNNING_LOCK, STOP, WORKERS = set(), threading.Lock(), threading.Event(), []
TESTKIT = "git.stit.tech/stit-core/golibinfra/testkit"
SHARED = CACHE / "shared"
# Серверы общего режима testkit: вызовы кита, которые его берут, порт, окружение и команда контейнера — как у
# контейнера Start* (testcontainers), строка готовности в логе (и сколько раз) или HTTP-проба.
SERVERS = {
    "postgres": ("PostgresImage", ("StartPostgres", "MigrationLadder"), "5432",
                 ["-e", "POSTGRES_USER=test", "-e", "POSTGRES_PASSWORD=test", "-e", "POSTGRES_DB=test"],
                 ["postgres", "-c", "fsync=off"],
                 ("database system is ready to accept connections", 2)),
    "nats": ("NATSImage", ("StartNATS",), "4222", [], ["-js"], ("Server is ready", 1)),
    "redis": ("RedisImage", ("StartRedis",), "6379", [], [], ("Ready to accept connections", 1)),
    "minio": ("MinIOImage", ("StartMinIO",), "9000", ["-e", "MINIO_ROOT_USER=minioadmin", "-e", "MINIO_ROOT_PASSWORD=minioadmin"],
              ["server", "/data"], ("/minio/health/cluster", 0)),
}
POOL = 2
READY_TIMEOUT = 120


def fail(msg):
    print(f"FAIL: {msg}")
    sys.exit(1)


def git(*args, cwd=ROOT):
    return subprocess.run(["git", *args], cwd=cwd, capture_output=True, check=True, env=ENV).stdout.decode()


def kill(p):
    """Группа процессов команды — SIGKILL; группы, которая уже вышла, нет — и снимать нечего."""
    with contextlib.suppress(ProcessLookupError):
        os.killpg(p.pid, signal.SIGKILL)


def run(cmd, wt=WT):
    """Код выхода и вывод команды в worktree wt; None — таймаут. Команда — своя группа процессов: таймаут и обрыв
    прогона снимают её целиком, с тестовыми бинарями go test, а не один go; её временные файлы go (GOTMPDIR — сборка и
    бинари тестов) уходят вместе с ней."""
    TMP.mkdir(parents=True, exist_ok=True)
    tmp = tempfile.mkdtemp(dir=TMP)
    env = dict(ENV, GOTMPDIR=tmp)
    with RUNNING_LOCK:
        if STOP.is_set():
            shutil.rmtree(tmp, ignore_errors=True)
            return -1, "STOPPED"
        p = subprocess.Popen(cmd, cwd=wt, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, errors="replace",
                             env=env, start_new_session=True)
        RUNNING.add(p)
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
        with RUNNING_LOCK:
            RUNNING.discard(p)
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


def apply(m, wt=WT):
    for e in m["edits"]:
        path = wt / e["path"]
        text = path.read_text()
        found = list(anchor(e["before"]).finditer(text))
        if len(found) != 1:
            return f"«было» в {e['path']} после предыдущей правки: вхождений {len(found)}"
        after = "\n".join(e["after"]).lstrip()
        path.write_text(text[: found[0].start()] + after + text[found[0].end():])
    return ""


def reset(head, wt=WT):
    """Worktree исполнителя — ровно снимок дерева разработчика: правка прошлой мутации, и оборванной, уходит. Каталог, который
    не worktree этого репо (репо перенесли или скопировали — он ведёт в прежний; первое создание оборвалось — он без
    .git), раннер создаёт заново. -f дважды: запись worktree, чьё создание оборвалось, остаётся под замком, и без него git
    отказал бы и в add, и в remove, а prune её не снимает."""
    if not own_worktree(wt):
        shutil.rmtree(wt, ignore_errors=True)
        git("worktree", "prune")
        git("worktree", "add", "-q", "-f", "-f", "--detach", str(wt), head)
    git("checkout", "-q", "-f", "--detach", head, cwd=wt)
    git("clean", "-q", "-fd", cwd=wt)


def own_worktree(wt=WT):
    if not wt.is_dir():
        return False
    p = subprocess.run(["git", "rev-parse", "--show-toplevel", "--path-format=absolute", "--git-common-dir"], cwd=wt,
                       capture_output=True, text=True, env=ENV)
    common = git("rev-parse", "--path-format=absolute", "--git-common-dir").strip()
    return p.returncode == 0 and p.stdout.splitlines() == [str(wt), common]


def trees(n, head):
    """Worktree исполнителей — снимок: первый — .cache/mutate/wt, k-й — wt-k; лишние прошлых прогонов с большим числом
    исполнителей снимаются. Создание — по очереди: git worktree prune одного не снимет недостроенный другого."""
    for extra in CACHE.glob("wt-*"):
        if not extra.name[3:].isdigit() or int(extra.name[3:]) > n:
            if own_worktree(extra):
                git("worktree", "remove", "-f", "-f", str(extra))
            shutil.rmtree(extra, ignore_errors=True)
    git("worktree", "prune")
    wts = [WT] + [CACHE / f"wt-{k}" for k in range(2, n + 1)]
    for wt in wts:
        reset(head, wt)
    return wts


def jobs(todo):
    """Число исполнителей и откуда оно: MUTATE_JOBS — явно; по умолчанию — половина ядер слота (GOMAXPROCS, без него — ядра
    машины) и не больше, чем помещается в доступную память по JOB_MEMORY_GB сверх запаса, и не больше мутаций."""
    raw = os.environ.get("MUTATE_JOBS", "")
    if raw:
        if not (raw.isascii() and raw.isdigit()) or int(raw) < 1:
            fail(f"MUTATE_JOBS={raw}: число исполнителей — целое ≥ 1")
        n, why = int(raw), "MUTATE_JOBS"
    else:
        cores = int(os.environ.get("GOMAXPROCS", "") or os.cpu_count() or 1)
        avail = 0.0
        with contextlib.suppress(OSError, ValueError, IndexError):
            line = next((x for x in pathlib.Path("/proc/meminfo").read_text().splitlines() if x.startswith("MemAvailable:")),
                        "")
            avail = int(line.split()[1]) / 2**20
        fits = int((avail - MEMORY_RESERVE_GB) / JOB_MEMORY_GB) if avail else cores // 2
        n = max(1, min(cores // 2, fits))
        why = (f"половина {cores} ядер, доступно {avail:.1f} ГБ: по {JOB_MEMORY_GB:g} ГБ на исполнителя и "
               f"{MEMORY_RESERVE_GB:g} ГБ запаса")
    return max(1, min(n, len(todo))), why


def failed(out):
    tests = re.findall(r"^\s*--- FAIL: (\S+)", out, re.M)
    top = [t for t in tests if "/" not in t] or tests
    pkgs = re.findall(r"^FAIL\s+(\S+)", out, re.M)
    return ", ".join(list(dict.fromkeys(top))[:6]) or ", ".join(list(dict.fromkeys(pkgs))[:4]) or out.strip()[-200:]


def prepare(m, log, wt=WT):
    """Правка мутации в worktree wt и сборка: вердикт stale или invalid и подробность, или None — собралась."""
    reason = stale(m, wt) or apply(m, wt)
    if reason:
        return "stale", reason
    paths = [e["path"] for e in m["edits"]]
    steps = []
    # Файл — один раз: gofumpt правит файлы параллельно, и две правки одного файла читали бы его, пока он пишется.
    gofiles = list(dict.fromkeys(p for p in paths if p.endswith(".go")))
    if gofiles:
        steps.append(("gofumpt", ["gofumpt", "-extra", "-w", *gofiles]))
    if any(p.endswith(".sql") for p in paths):
        steps.append(("sqlc", ["sqlc", "generate"]))
    if any(any(h in p for h in HTTP_PATHS) for p in paths):
        steps.append(("openapi", ["go", "test", "./cmd/api", "-run", "^TestOpenAPISpec$", "-count=1", "-args", "-update"]))
    # Компиляция без линковки (go list -export) ловит ошибку компиляции любого пакета, и программ cmd/. Линковка
    # добавляет одну — нет func main, — и её даёт только правка файла самой программы (ассемблера и go:linkname в коде
    # нет): такая программа линкуется, остальные — нет; бинари тестов линкует go test.
    steps.append(("build", ["go", "list", "-export", "-f", '{{""}}', "./..."]))
    for d in dict.fromkeys(pathlib.PurePosixPath(p).parent.as_posix() for p in gofiles
                           if not p.endswith("_test.go") and MAIN.search((wt / p).read_text())):
        steps.append(("build", ["go", "build", "-o", os.devnull, pattern(d)]))
    for name, cmd in steps:
        code, out = run(cmd, wt)
        log.append(f"$ {' '.join(cmd)}  (exit {code})\n{out}")
        if code != 0:
            return "invalid", f"{name}: {out.strip()[-200:]}"
    log.append("$ git diff\n" + git("diff", cwd=wt))
    return None


def pattern(d):
    """Шаблон go для пакета каталога d (путь от корня worktree)."""
    return "." if d == "." else f"./{d}"


def golist(*args):
    """Вывод go list в worktree раннера; отказ — конец прогона."""
    p = subprocess.run(["go", "list", *args], cwd=WT, capture_output=True, text=True, errors="replace", env=ENV)
    if p.returncode != 0:
        fail(f"go list {' '.join(args)}: {p.stderr.strip()[-300:]}")
    return p.stdout


def stages():
    """Тесты мутации по порядку — (стадия, команда) — и снят ли второй прогон archtest: юнит-тесты всех пакетов, кроме
    internal/archtest, затем юнит + integration их же, затем archtest без тестов раннера — юнит-стадии, и с тегом
    integration, если у его теста с тегом другие файлы. archtest с теми же файлами теста и зависимостей — тот же бинарь (тег меняет в нём лишь
    строку build info): второй раз не гоняется. Модуль без internal/archtest — стадия целиком, go test ./..."""
    if not (WT / ARCHTEST).is_dir():
        return [("unit", ["go", "test", "./..."]), ("integration", ["go", "test", "-tags=integration", "./..."])], False
    wt, arch = WT.resolve(), (WT / ARCHTEST).resolve()
    rest, last, files, once = [], [], None, False
    for stage, tags in (("unit", []), ("integration", ["-tags=integration"])):
        dirs = [pathlib.Path(d).resolve() for d in golist(*tags, "-f", "{{.Dir}}", "./...").splitlines() if d]
        pkgs = [pattern(d.relative_to(wt).as_posix()) for d in dirs if d != arch]
        if pkgs:
            rest.append((stage, ["go", "test", *tags, *pkgs]))
        if arch in dirs:
            seen, files = files, golist(*tags, "-deps", "-test", "-f", TEST_FILES, f"./{ARCHTEST}")
            if files == seen:
                once = True
            else:
                last.append((stage, ["go", "test", *tags, "-skip", RUNNER_TESTS, f"./{ARCHTEST}"]))
    return rest + last, once


def docker(*args):
    """Вывод команды docker; отказ — RuntimeError с её выводом."""
    p = subprocess.run(["docker", *args], capture_output=True, text=True, errors="replace", env=ENV)
    if p.returncode != 0:
        raise RuntimeError(f"docker {' '.join(args[:2])}: {(p.stderr or p.stdout).strip()[-300:]}")
    return p.stdout.strip()


REPO = hashlib.sha256(str(ROOT).encode()).hexdigest()[:8]
# Прогон — метка mutate.run=<id> и имя mutate-<id>-<вид>-<номер> контейнеров: уборка в конце находит их по метке;
# mutate.repo — репо: оставленные убитым раннером снимает следующий прогон.
RUN = f"{REPO}-{os.getpid()}"


def sweep(scope="mutate.repo=" + REPO):
    """Контейнеры общего режима по метке scope — долой: по умолчанию все контейнеры прогонов этого репо."""
    ids = docker("ps", "-aq", "--filter", f"label={scope}").split()
    if ids:
        docker("rm", "-f", "-v", *ids)


def kits():
    """Виды общих серверов, которые зовут тесты worktree (Start* кита), и образы кита — или пусто: кит без общего режима
    (golibinfra старше SharedEnv), модуль без кита или тестов с контейнерами."""
    calls = re.compile(r"\btestkit\.(" + "|".join(c for k in SERVERS.values() for c in k[1]) + r")\(")
    used = set()
    for f in git("ls-files", "*.go", cwd=WT).splitlines():
        for call in calls.findall((WT / f).read_text(errors="replace")):
            used |= {kind for kind, spec in SERVERS.items() if call in spec[1]}
    if not used:
        return {}
    p = subprocess.run(["go", "doc", "-all", TESTKIT], cwd=WT, capture_output=True, text=True, errors="replace", env=ENV)
    consts = dict(re.findall(r'^\s*(?:const\s+)?(\w+)\s*=\s*"([^"]+)"', p.stdout, re.M)) if p.returncode == 0 else {}
    if consts.get("SharedEnv") != "TESTKIT_SHARED":
        return {}
    return {kind: consts[SERVERS[kind][0]] for kind in used if SERVERS[kind][0] in consts}


# Проба — мимо http_proxy окружения: urllib, в отличие от Go, шлёт в прокси и 127.0.0.1.
DIRECT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def ready(cid, kind, port):
    """Ждёт готовности сервера: строка лога нужное число раз или HTTP-проба на проброшенном порту."""
    probe, times = SERVERS[kind][5]
    deadline = time.monotonic() + READY_TIMEOUT
    while time.monotonic() < deadline:
        if times:
            out = subprocess.run(["docker", "logs", cid], capture_output=True, text=True, errors="replace", env=ENV)
            if (out.stdout + out.stderr).count(probe) >= times:
                return
        else:
            with contextlib.suppress(OSError):
                with DIRECT.open(f"http://{port}{probe}", timeout=2) as r:
                    if r.status == 200:
                        return
        time.sleep(0.2)
    raise RuntimeError(f"{kind} {cid[:12]}: нет готовности за {READY_TIMEOUT} с")


def shared_up(started, jobs=1):
    """Серверы общего режима testkit — один раз на прогон: Postgres один, NATS, Redis и MinIO — пулом не меньше числа
    исполнителей; их файл — в TESTKIT_SHARED команд прогона. started получает id контейнеров сразу после запуска, а до
    первого — отметку запуска: уборка не упустит и тот, чей id docker run не успел отдать."""
    images = kits()
    if not images:
        return
    started["launched"] = True
    sweep()
    locks = SHARED / "locks"
    locks.mkdir(parents=True, exist_ok=True)
    servers, waits = {"locks": str(locks)}, []
    for kind in ("postgres", "nats", "redis", "minio"):
        if kind not in images:
            continue
        _, _, port, env, cmd, _ = SERVERS[kind]
        if kind == "postgres":
            cmd = [*cmd, "-c", f"max_connections={max(500, 250 * jobs)}"]
        for i in range(1 if kind == "postgres" else max(POOL, jobs)):
            cid = docker("run", "-d", "--name", f"mutate-{RUN}-{kind}-{i}", "--label", f"mutate.run={RUN}",
                         "--label", f"mutate.repo={REPO}", "-p", f"127.0.0.1::{port}", *env, images[kind], *cmd)
            started["ids"].append(cid)
            host = docker("port", cid, f"{port}/tcp").splitlines()[0]
            waits.append((cid, kind, host))
            if kind == "postgres":
                servers[kind] = {"image": images[kind], "url": f"postgres://test:test@{host}/test?sslmode=disable"}
            elif kind == "minio":
                servers.setdefault(kind, []).append({"image": images[kind], "endpoint": host, "access_key": "minioadmin",
                                                     "secret_key": "minioadmin"})
            else:
                servers.setdefault(kind, []).append({"image": images[kind], "url": f"{kind}://{host}"})
            (locks / f"{kind}-{i}.lock").touch()
    path = SHARED / "servers.json"
    path.write_text(json.dumps(servers, indent=1))
    for cid, kind, host in waits:
        ready(cid, kind, host)
    ENV["TESTKIT_SHARED"] = str(path)
    counts = [f"{k} ×{len(v)}" if isinstance(v, list) else k for k, v in servers.items() if k != "locks"]
    print(f"→ серверы integration — один раз на прогон (TESTKIT_SHARED): {', '.join(counts)}")


def shared_down(started):
    """Серверы общего режима прогона — долой с их анонимными томами: по id и по метке прогона."""
    ENV.pop("TESTKIT_SHARED", None)
    if not started["launched"]:
        return
    if started["ids"]:
        with contextlib.suppress(RuntimeError, OSError):
            docker("rm", "-f", "-v", *started["ids"])
    with contextlib.suppress(RuntimeError, OSError):
        sweep(f"mutate.run={RUN}")
    with contextlib.suppress(OSError):
        (SHARED / "servers.json").unlink()


def mutate(m, log, plan, wt=WT):
    """Вердикт мутации: unit, integration (убита), survived, invalid, timeout — и подробность."""
    verdict = prepare(m, log, wt)
    if verdict:
        return verdict
    for stage, cmd in plan:
        t0 = time.time()
        code, out = run(cmd, wt)
        log.append(f"$ {' '.join(cmd)}  ({int(time.time() - t0)}s, exit {code})\n{out}")
        if code is None:
            return "timeout", stage
        if code != 0:
            return stage, failed(out)
    return "survived", ""


KILLED = ("unit", "integration")
TEST_FUNC = re.compile(r"^func ((?:Test|Fuzz)\w*)\(", re.M)


def key(m):
    """Ключ результата мутации: её блок и файлы, которые она правит, в worktree (снимок)."""
    h = hashlib.sha256(json.dumps(m, sort_keys=True, ensure_ascii=False).encode())
    for path in sorted({e["path"] for e in m["edits"]}):
        f = WT / path
        h.update(b"\0" + path.encode() + b"\0" + (f.read_bytes() if f.is_file() else b"-"))
    return h.hexdigest()[:20]


def test_names():
    """Тесты и fuzz-цели worktree: убитая мутация переиспользуется, пока на месте тест, который её убил."""
    names = set()
    for f in git("ls-files", "*_test.go", cwd=WT).splitlines():
        names |= set(TEST_FUNC.findall((WT / f).read_text()))
    return names


def snapshot():
    """Ревизия прогона и HEAD: снимок рабочего дерева — коммит поверх HEAD без ссылки; без правок — сам HEAD. Снимок
    собирает копия индекса (git add -u: отслеживаемые файлы и новые в индексе, и git add -N, — без неотслеживаемых):
    индекс, дерево и ссылки разработчика остаются как были. git stash create здесь не годится — на записи git add -N
    он отказывает."""
    head = git("rev-parse", "HEAD").strip()
    with tempfile.TemporaryDirectory() as tmp:
        env = {**ENV, "GIT_INDEX_FILE": os.path.join(tmp, "index")}
        shutil.copyfile(ROOT / git("rev-parse", "--git-path", "index").strip(), env["GIT_INDEX_FILE"])
        subprocess.run(["git", "add", "-u"], cwd=ROOT, capture_output=True, check=True, env=env)
        tree = subprocess.run(["git", "write-tree"], cwd=ROOT, capture_output=True, check=True, env=env).stdout.decode().strip()
    if tree == git("rev-parse", "HEAD^{tree}").strip():
        return head, head
    # Снимок без ссылки — не коммит разработчика: подпись своя, git без user.name его не отвергает.
    ident = {"GIT_AUTHOR_NAME": "make mutate", "GIT_AUTHOR_EMAIL": "mutate@localhost",
             "GIT_COMMITTER_NAME": "make mutate", "GIT_COMMITTER_EMAIL": "mutate@localhost"}
    commit = subprocess.run(["git", "commit-tree", tree, "-p", head, "-m", "make mutate: снимок рабочего дерева"], cwd=ROOT,
                            capture_output=True, check=True, env={**ENV, **ident})
    return commit.stdout.decode().strip(), head


def main(args):
    flags = {a for a in args if a in ("--strict", "--fresh", "--build", "--check", "--anchors")}
    strict, fresh = "--strict" in flags, "--fresh" in flags
    args = [a for a in args if a not in flags]
    if "--anchors" in flags or "--check" in flags:
        check(args, strict)
        if "--anchors" in flags:
            return 0
        flags.add("--build")

    CACHE.mkdir(parents=True, exist_ok=True)
    lock = open(CACHE / "lock", "w")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except OSError:
        fail("make mutate уже идёт в этом дереве")
    if subprocess.run(["go", "env", "GOFLAGS"], capture_output=True, text=True, env=ENV).stdout.strip():
        fail("make mutate не запускается с GOFLAGS — тесты шли бы не так, как в гейте")
    # Временные файлы стадий прогона, оборванного SIGKILL раннера: под замком этого дерева они ничьи.
    shutil.rmtree(TMP, ignore_errors=True)
    # SIGTERM и SIGHUP (kill, закрытый терминал) — как Ctrl-C: прогон выходит через откат worktree.
    for sig in (signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, lambda signum, _: sys.exit(128 + signum))

    head, base = snapshot()
    tree = git("rev-parse", f"{head}^{{tree}}").strip()
    if head != base:
        print(f"→ мутации идут по рабочему дереву: снимок {head[:7]} поверх HEAD {base[:7]}")
    untracked = git("ls-files", "--others", "--exclude-standard").split()
    if untracked:
        print(f"→ неотслеживаемые файлы в прогон не входят (git add): {' '.join(untracked[:5])}"
              + (" …" if len(untracked) > 5 else ""))
    reset(head)
    mutants = parse((WT / SPEC).read_text())
    by_id = {m["id"]: m for m in mutants}
    unknown = [a for a in args if a not in by_id]
    if unknown:
        fail("нет таких мутаций: " + " ".join(unknown))
    selected = [by_id[a] for a in args] if args else mutants
    if "--build" in flags:
        return build(selected, head)
    bare = [] if args else untested(mutants, WT, git("ls-files", cwd=WT).splitlines())

    results = CACHE / "results.tsv"
    for old in CACHE.glob("*.tsv"):
        if old != results:
            old.unlink()
    logs = CACHE / "logs"
    logs.mkdir(exist_ok=True)
    keys = {m["id"]: key(m) for m in mutants}
    done, reused, last = {}, set(), {}
    if results.exists() and not args and not fresh:
        for line in results.read_text().splitlines():
            k, mid, verdict, _, detail, at = (line.split("\t") + [""] * 6)[:6]
            if keys.get(mid) == k:
                last[mid] = (verdict, detail, at)
    tests = test_names() if last and not strict else set()
    for mid, (verdict, detail, at) in last.items():
        if strict:
            # Ключ не видит теста, ослабленного в другом файле: шаг релиза берёт только вердикты этого же снимка, а
            # несобравшиеся и таймауты — от окружения тоже (память, диск) — гоняет заново.
            take = at == tree and verdict not in ("invalid", "timeout")
        else:
            take = verdict in KILLED and bool({t.split("/")[0] for t in detail.split(", ")} & tests)
        if take:
            done[mid] = (verdict, detail)
            reused.add(mid)
    todo = [m for m in selected if m["id"] not in reused]
    if reused and strict:
        print(f"→ вердикты прогонов того же снимка: {len(reused)} — не гоняются (строгий прогон продолжается с места; "
              f"A=--fresh — все заново)")
    elif reused:
        print(f"→ убитые прошлыми прогонами с тем же ключом (блок и его файлы) и тестом на месте: {len(reused)} — не "
              f"гоняются (A=--fresh — все заново, A=--strict — все, кроме вердиктов того же снимка)")

    plan, once = stages() if todo else ([], False)
    n, why = jobs(todo) if todo else (1, "")
    started, wts = {"launched": False, "ids": []}, [WT]
    try:
        if todo:
            try:
                shared_up(started, n)
            except (RuntimeError, OSError) as e:
                fail(f"серверы integration: {e}")
            wts = trees(n, head)
            print(f"→ исполнителей: {n} ({why})" + (", у каждого — свой worktree" if n > 1 else ""))
            print(f"→ базовый прогон: go test и go test -tags=integration по {head[:7]} ({len(todo)} мутаций впереди)")
            if once:
                print(f"→ {ARCHTEST} гоняется последним, после integration, и один раз: с тегом integration у его теста "
                      f"те же файлы")
            for _, cmd in plan:
                code, out = run(cmd)
                if code != 0:
                    print(out[-3000:])
                    fail(f"базовый прогон {' '.join(cmd)} красный — мутации на красных тестах без смысла")
            execute(todo, plan, head, wts, keys, results, logs, done, tree)
    finally:
        # Уборку второй Ctrl-C или SIGTERM не обрывает: иначе серверы прогона жили бы до следующего.
        for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            signal.signal(sig, signal.SIG_IGN)
        try:
            stop(head, wts)
        finally:
            shared_down(started)
    # Результаты — по ключам текущей спецификации: строка прежнего блока или кода больше не нужна, последняя — свежее.
    if results.exists():
        current = {}
        for line in results.read_text().splitlines():
            k, mid = (line.split("\t") + ["", ""])[:2]
            if keys.get(mid) == k:
                current[mid] = line
        results.write_text("".join(current[m["id"]] + "\n" for m in mutants if m["id"] in current))
    return summary(selected, done, head, strict, bare)


def execute(todo, plan, head, wts, keys, results, logs, done, tree):
    """Мутации — исполнителям: общая очередь в порядке спецификации, исполнитель — в своём worktree. Базовый прогон —
    в первом: деревья одинаковы, а кеш go test привязан к каталогу пакета — первая мутация исполнителя греет его дерево.
    Результат пишется в results.tsv сразу, как готов (оборванный прогон продолжится с места), строки хода — в порядке
    спецификации."""
    work, out = queue.Queue(), queue.Queue()
    for item in enumerate(todo, 1):
        work.put(item)

    def worker(wt):
        while not STOP.is_set():
            try:
                k, m = work.get_nowait()
            except queue.Empty:
                return
            t0, log = time.time(), []
            try:
                verdict, detail = mutate(m, log, plan, wt)
            except BaseException as e:
                verdict, detail = None, repr(e)
            if STOP.is_set():
                return
            try:
                reset(head, wt)
            except BaseException as e:
                verdict, detail = None, f"сброс {wt.name}: {e!r}"
            out.put((k, m, verdict, detail, int(time.time() - t0), log))
            if verdict is None:
                return

    threads = [threading.Thread(target=worker, args=(wt,), daemon=True) for wt in wts]
    WORKERS.extend(threads)
    for t in threads:
        t.start()
    pending, nxt = {}, 1
    while nxt <= len(todo):
        try:
            k, m, verdict, detail, dt, log = out.get(timeout=0.5)
        except queue.Empty:
            if not any(t.is_alive() for t in threads) and out.empty():
                fail("исполнители вышли раньше очереди")
            continue
        if verdict is None:
            raise RuntimeError(f"исполнитель на {m['id']}: {detail}")
        detail = " ".join(detail.split())
        (logs / f"{m['id']}.log").write_text("\n\n".join(log))
        done[m["id"]] = (verdict, detail)
        with results.open("a") as f:
            f.write("\t".join([keys[m["id"]], m["id"], verdict, f"{dt}s", detail, tree]) + "\n")
        pending[k] = f"[{k}/{len(todo)}] {m['id']} {m['layer']} — {verdict} {dt}s {detail[:120]}".rstrip()
        while nxt in pending:
            print(pending.pop(nxt), flush=True)
            nxt += 1
    for t in threads:
        t.join()


def stop(head, wts):
    """Конец и обрыв прогона: новых команд нет, идущие сняты с группами процессов, worktree исполнителей — снова снимок."""
    STOP.set()
    with RUNNING_LOCK:
        for p in list(RUNNING):
            kill(p)
    for t in WORKERS:
        t.join(timeout=60)
    for wt in wts:
        with contextlib.suppress(subprocess.CalledProcessError, OSError):
            reset(head, wt)


def build(selected, head):
    """A=--build: каждая мутация только собирается — правка, gofumpt, sqlc и openapi.json по её путям, go build ./... —
    без тестов и Docker: несобирающиеся правки за минуты. Код 1 — есть несобравшиеся или устаревшие якоря."""
    print(f"→ сборка мутаций без тестов по {head[:7]} ({len(selected)})")
    bad = []
    for k, m in enumerate(selected, 1):
        t0 = time.time()
        try:
            verdict, detail = prepare(m, []) or ("built", "")
        finally:
            reset(head)
        detail = " ".join(detail.split())
        print(f"[{k}/{len(selected)}] {m['id']} {m['layer']} — {verdict} {int(time.time() - t0)}s {detail[:120]}".rstrip(),
              flush=True)
        if verdict != "built":
            bad.append((m, verdict, detail))
    print(f"\nСборка ({head[:7]}, мутаций {len(selected)}): собрались {len(selected) - len(bad)}")
    for verdict, title, hint in (("invalid", "не собирается", "поправь правку «стало»"),
                                 ("stale", "устаревший якорь", "перепиши блок под код")):
        group = [(m, detail) for m, v, detail in bad if v == verdict]
        if group:
            print(f"{title} ({len(group)}): {hint}")
            for m, detail in group:
                print(f"  {m['id']} {m['layer']} — {m['note']}" + (f" [{detail}]" if detail else ""))
    if bad:
        fail(f"мутаций, которые не собираются или не применяются: {len(bad)}")
    print("OK: все мутации собираются")
    return 0


def check(ids, strict):
    """A=--anchors и первая часть A=--check: якоря спецификации рабочего дерева (с ids — этих мутаций) и файлы кода без
    мутаций."""
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
    print(f"\nИтог ({head[:7]}, мутаций {len(selected)}): убиты {len(unit) + len(integration)} — юнит {len(unit)}, "
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
