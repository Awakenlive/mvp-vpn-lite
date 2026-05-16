# Подробное описание проекта `mvp-vpn-lite`

Этот документ описывает, что было реализовано в проекте, как устроена
архитектура, какие этапы разработки были пройдены, какие проверки выполнялись
и какие ограничения остались. Текст можно использовать как основу для курсовой
работы.

## 1. Общая идея проекта

`mvp-vpn-lite` - это прототип легкого VPN-туннеля на языке Go. Основная идея
проекта состоит в том, чтобы передавать IPv4-пакеты через QUIC-соединения.
QUIC выбран как транспортный протокол, потому что он работает поверх UDP,
использует TLS 1.3, поддерживает потоки и хорошо подходит для экспериментов с
несколькими сетевыми путями.

Проект реализован как MVP, то есть минимально жизнеспособный прототип. Он не
является готовым промышленным VPN-решением, но демонстрирует ключевые принципы:

- создание IPv4/ICMP-пакетов в коде;
- упаковка пакетов в собственный простой frame-протокол;
- передача frame-пакетов через QUIC;
- работа с одним или двумя QUIC-путями;
- round-robin распределение пакетов между путями;
- работа с Linux TUN-интерфейсами;
- восстановление упавших путей;
- базовая телеметрия RX/TX/drop/error;
- JSON-формат статистики для машинной обработки логов;
- trusted TLS и mutual TLS;
- простая packet policy для TUN-пакетов;
- настройка через CLI-флаги и переменные окружения;
- подготовка helper-скриптов и systemd-примеров.

На текущем состоянии проект умеет работать в двух основных режимах:

1. Демо-режим без TUN: клиент сам создает ICMP echo request, сервер сам строит
   ICMP echo reply. Этот режим нужен для простой проверки QUIC-транспорта без
   root-прав.
2. TUN-режим: клиент и сервер читают и пишут реальные IPv4-пакеты через Linux
   TUN-устройства. В этом режиме можно проверять туннель обычным `ping`.

## 2. Структура проекта

Проект разделен на небольшие пакеты, чтобы каждую часть можно было тестировать
отдельно:

- `cmd/client` - CLI-приложение клиента.
- `cmd/server` - CLI-приложение сервера.
- `internal/packet` - работа с IPv4 и ICMP.
- `internal/quictransport` - QUIC-клиент, QUIC-сервер, framing, TUN pump,
  failover и reconnect.
- `internal/multipath` - round-robin scheduler.
- `internal/tun` - открытие Linux TUN-устройства.
- `internal/stats` - счетчики RX/TX/drop/error.
- `internal/envconfig` - чтение значений из переменных окружения.
- `scripts` - shell-скрипты для настройки и очистки TUN-интерфейсов.
- `examples/env` - примеры environment-файлов.
- `examples/systemd` - примеры systemd unit-файлов.
- `docs` - архитектура, протокол, тестирование, эксплуатация и этот отчет.

Такое разделение было выбрано специально: низкоуровневый код пакетов не зависит
от QUIC, QUIC-логика не зависит напрямую от CLI, а TUN-поведение тестируется
через интерфейсы и fake-объекты.

## 3. Этапы разработки

### Этап 1. Каркас Go-проекта

Сначала был создан Go-модуль `mvp-vpn-lite`. В `go.mod` подключены основные
зависимости:

- `github.com/quic-go/quic-go` - реализация QUIC для Go;
- `golang.org/x/sys` - доступ к Linux syscall, включая TUN ioctl.

Были выделены отдельные пакеты под транспорт, пакеты, мультипуть, TUN,
статистику и конфигурацию. Это позволило писать тесты по слоям, не смешивая
все в одном большом `main.go`.

### Этап 2. Работа с IPv4 и ICMP

В пакете `internal/packet` реализована минимальная работа с IPv4 и ICMP echo:

- разбор IPv4-пакета;
- проверка версии IPv4;
- проверка длины заголовка;
- проверка total length;
- проверка протокола ICMP;
- извлечение source/destination IP;
- разбор ICMP echo-пакета;
- построение ICMP echo request;
- построение ICMP echo reply;
- расчет стандартной Internet checksum.

В демо-режиме это позволяет клиенту создать настоящий IPv4-пакет с ICMP echo
request, а серверу - разобрать его и вернуть корректный ICMP echo reply.

Важно, что reply не является строкой или условным JSON-сообщением. Это именно
сырые байты IPv4-пакета, у которого пересчитаны IPv4 и ICMP checksums.

### Этап 3. Простой frame-протокол поверх QUIC stream

В пакете `internal/quictransport` реализован framing:

```text
4 байта length, big-endian
N байт payload
```

Payload - это raw IPv4 packet. Максимальный размер frame установлен в `65535`
байт. Нулевые payload запрещены.

Такой формат нужен потому, что QUIC stream - это поток байтов. Если просто
писать пакеты подряд, принимающая сторона не будет знать, где заканчивается
один пакет и начинается следующий. Поэтому перед каждым пакетом передается
длина.

Реализованы две функции:

- `WriteFrame(w, payload)` - записывает длину и пакет;
- `ReadFrame(r)` - читает длину, проверяет ее и читает payload.

### Этап 4. QUIC demo client/server

После packet layer и frame layer был реализован базовый QUIC-клиент и
QUIC-сервер.

Сервер:

- поднимает один или два QUIC listener;
- принимает QUIC connection;
- принимает stream;
- читает frame;
- пытается построить ICMP echo reply;
- отправляет reply обратно frame-ом.

Клиент:

- подключается к одному или двум адресам сервера;
- открывает stream на каждом QUIC connection;
- создает ICMP echo request;
- отправляет request;
- читает reply;
- проверяет IPv4 checksum;
- проверяет ICMP checksum;
- проверяет source/destination IP;
- проверяет identifier, sequence и payload.

Этот режим позволил быстро проверять транспорт без root-прав и без TUN.

### Этап 5. Multipath и round-robin

Была добавлена поддержка двух путей:

- `server0` / `listen0` - path 0;
- `server1` / `listen1` - path 1.

Каждый путь - это отдельное QUIC-соединение. Для выбора пути реализован простой
round-robin scheduler в пакете `internal/multipath`.

Принцип работы:

1. Есть `N` активных путей.
2. Scheduler возвращает индекс следующего пути.
3. После последнего пути индекс возвращается к нулю.

Например, для двух путей последовательность будет:

```text
0, 1, 0, 1, 0, 1, ...
```

На уровне демо-клиента это видно по логам: четыре ICMP-запроса уходят по путям
`0, 1, 0, 1`.

### Этап 6. Linux TUN на стороне клиента

Следующий шаг - работа не только с синтетическими ICMP-пакетами, но и с
реальным сетевым интерфейсом.

В Linux TUN-интерфейс выглядит как виртуальная сетевое устройство. Когда
приложение читает из TUN fd, оно получает IP-пакеты, которые ядро направило в
этот интерфейс. Когда приложение пишет байты в TUN fd, ядро воспринимает их как
входящие IP-пакеты с этого интерфейса.

В проекте был создан пакет `internal/tun`.

На Linux используется:

- `/dev/net/tun`;
- `TUNSETIFF`;
- `IFF_TUN`;
- `IFF_NO_PI`.

Флаг `IFF_NO_PI` важен: без него ядро добавляло бы дополнительный packet-info
prefix. С этим флагом приложение читает и пишет чистые IPv4-пакеты.

Для клиента добавлен режим `-tun`. В этом режиме клиент:

1. открывает TUN-устройство, например `mvpvpn0`;
2. читает из него raw IPv4 packet;
3. выбирает активный QUIC path;
4. отправляет пакет через `WriteFrame`;
5. читает ответы из QUIC path receiver-горутин;
6. пишет полученные raw packets обратно в TUN.

### Этап 7. Linux TUN на стороне сервера

После клиентского TUN был добавлен полноценный server-side TUN mode.

В режиме `cmd/server -tun` сервер больше не строит ICMP echo reply сам. Вместо
этого он работает как packet forwarder:

1. принимает frame от клиента;
2. достает raw IPv4 packet;
3. пишет его в серверный TUN, например `mvpvpns0`;
4. читает ответные пакеты из `mvpvpns0`;
5. отправляет ответы обратно клиенту через активные QUIC streams.

Такой режим уже ближе к реальному VPN: обе стороны имеют виртуальные
интерфейсы, а приложение прокачивает IP-пакеты между ними через QUIC.

### Этап 8. Helper-скрипты для TUN

Так как TUN-интерфейсы требуют root-прав или `CAP_NET_ADMIN`, были добавлены
shell-скрипты:

- `scripts/setup-client.sh`;
- `scripts/cleanup-client.sh`;
- `scripts/setup-server.sh`;
- `scripts/cleanup-server.sh`;
- `scripts/lib-tun.sh`.

Клиентский helper по умолчанию:

- создает `mvpvpn0`;
- назначает `10.8.0.2/24`;
- выставляет MTU `1400`;
- добавляет route `10.8.0.1/32` через `mvpvpn0`.

Серверный helper по умолчанию:

- создает `mvpvpns0`;
- назначает `10.8.0.1/24`;
- выставляет MTU `1400`;
- optionally добавляет route, если задана переменная `ROUTE`.

Скрипты сделаны идемпотентными: их можно запускать повторно, они используют
`ip addr replace`, `ip route replace` и проверяют существование link.

Также реализован режим `DRY_RUN=1`, который печатает команды, но не изменяет
систему. Это удобно для тестов и для объяснения в курсовой, какие именно
команды выполняются.

### Этап 9. Статистика

Добавлен пакет `internal/stats`.

Он хранит:

- количество принятых пакетов;
- количество принятых байт;
- количество отправленных пакетов;
- количество отправленных байт;
- количество drop-событий;
- количество ошибок.

Счетчики реализованы через `atomic.Uint64`, поэтому их можно безопасно
увеличивать из разных горутин.

CLI получил флаг:

```text
-stats-interval
-stats-json
```

Если interval больше нуля, приложение периодически пишет snapshot в лог. При
завершении всегда выводится финальная статистика. По умолчанию она выводится в
читаемом text-формате, а с `-stats-json` - в JSON:

Пример финальной строки:

```text
stats final: rx=4 packets/160 bytes tx=4 packets/160 bytes dropped=0 errors=0
stats final: {"rx_packets":4,"rx_bytes":160,"tx_packets":4,"tx_bytes":160,"dropped_packets":0,"errors":0}
```

### Этап 10. TLS-режимы

QUIC всегда использует TLS. В проекте реализованы два варианта:

1. Demo mode: если сертификат не передан, сервер генерирует ephemeral
   self-signed certificate, а клиент без `-ca-cert` использует
   `InsecureSkipVerify`. Это удобно для локальной разработки.
2. Trusted mode: сервер запускается с `-tls-cert` и `-tls-key`, клиент получает
   `-ca-cert` и optional `-server-name`. В этом режиме клиент проверяет
   сертификат сервера.
3. Mutual TLS: сервер получает `-client-ca` и требует клиентский сертификат, а
   клиент запускается с `-client-cert` и `-client-key`.

Также задан ALPN:

```text
mvp-vpn-lite
```

Минимальная версия TLS - TLS 1.3.

### Этап 10.1. Packet policy для TUN

Для TUN-режима добавлен флаг:

```text
-tun-allow-cidr
```

Если он задан, endpoint проверяет каждый raw IPv4 packet перед forwarding.
Source IP и destination IP должны входить в указанный CIDR. Malformed packets,
non-IPv4 packets и packets за пределами CIDR drop-аются и учитываются в
stats. Это не полноценный firewall, но это уже минимальная защита от случайного
форвардинга чужих адресов.

### Этап 11. Reconnect и failover в TUN-клиенте

В TUN-режиме клиент должен переживать падение одного пути. Для этого была
реализована структура активных путей `clientPathSet`.

Она умеет:

- добавлять path;
- заменять path с тем же ID;
- удалять path по stream;
- выбирать следующий path round-robin;
- проверять, есть ли path с конкретным ID.

Если запись в QUIC stream ломается:

1. путь удаляется из active path set;
2. stream/connection закрываются;
3. для этого path записывается короткий cooldown;
4. текущий пакет пробуется отправить через следующий активный путь.

Cooldown нужен, чтобы только что упавший и восстановленный path не получал
трафик сразу же, если рядом есть другой здоровый active path. Это простая
модель health tracking: она не измеряет latency, но уменьшает flapping после
ошибок записи.

Если все пути пропали, TUN-пакеты временно drop-аются, а счетчик drops растет.

Для каждого настроенного path запускается reconnect goroutine. Она:

1. проверяет, есть ли path с нужным ID;
2. если path отсутствует, пытается заново dial-нуть сервер;
3. при неудаче ждет backoff delay;
4. delay растет от `reconnect-min` до `reconnect-max`;
5. при успешном reconnect backoff сбрасывается.

По умолчанию:

```text
reconnect-min = 1s
reconnect-max = 30s
```

Для тестов использовались ускоренные значения, например `200ms` и `2s`.

### Этап 12. Переменные окружения и systemd

Для удобного запуска как сервиса были добавлены environment defaults для всех
CLI-флагов.

Принцип:

- переменная окружения задает значение по умолчанию;
- явный CLI-флаг имеет приоритет над env.

Например:

```text
MVPVPN_CLIENT_SERVER0
MVPVPN_CLIENT_SERVER1
MVPVPN_CLIENT_TUN
MVPVPN_CLIENT_TUN_NAME
MVPVPN_CLIENT_RECONNECT_MIN
MVPVPN_CLIENT_RECONNECT_MAX
```

Для сервера:

```text
MVPVPN_SERVER_LISTEN0
MVPVPN_SERVER_LISTEN1
MVPVPN_SERVER_TUN
MVPVPN_SERVER_TUN_NAME
MVPVPN_SERVER_TLS_CERT
MVPVPN_SERVER_TLS_KEY
```

Также добавлены:

- `examples/env/client.env`;
- `examples/env/server.env`;
- `examples/systemd/mvp-vpn-lite-client.service`;
- `examples/systemd/mvp-vpn-lite-server.service`;
- `docs/operations.md`.

Systemd units используют:

- `EnvironmentFile`;
- `ExecStartPre` для настройки TUN;
- `ExecStart` для запуска daemon;
- `ExecStopPost` для cleanup;
- `CapabilityBoundingSet=CAP_NET_ADMIN`;
- `AmbientCapabilities=CAP_NET_ADMIN`.

Это показывает, как MVP можно запускать как Linux-сервис.

### Этап 13. Документация

Была подготовлена документация:

- `README.md` - быстрый старт, статус, ограничения, основные команды.
- `docs/architecture.md` - компоненты, packet flow, concurrency model.
- `docs/protocol.md` - QUIC, TLS, frame format, TUN-mode behavior.
- `docs/testing.md` - unit, smoke и manual checks.
- `docs/operations.md` - сборка, установка, env files, systemd, troubleshooting.
- `docs/coursework-summary.md` - подробный отчет для курсовой.

Документация покрывает не только запуск, но и внутреннюю архитектуру, что важно
для учебной работы.

### Этап 14. Исправление проблемы `read /dev/net/tun: not pollable`

Во время тяжелого root/netns stress testing была найдена нестабильность:

```text
read /dev/net/tun: not pollable
```

Симптом проявлялся не всегда. Он возникал при многократном создании и удалении
network namespaces, TUN-интерфейсов и процессов client/server.

Сначала была добавлена защитная обработка такой ошибки в TUN read loops:

- ошибка классифицируется как retryable;
- выполняется bounded retry;
- логирование ограничено, чтобы не заливать лог;
- после лимита ошибка все равно считается fatal.

Но затем выяснилось, что правильная причина ниже: Go 1.26 может проблемно
зарегистрировать `/dev/net/tun`, открытый через `os.OpenFile`, в runtime
network poller. В исходниках Go даже есть тест, где `/dev/net/tun` используется
как пример file descriptor в плохом poller-состоянии.

Поэтому `internal/tun/tun_linux.go` был исправлен:

- вместо `os.OpenFile("/dev/net/tun", ...)` используется `unix.Open`;
- fd открывается с `O_RDWR | O_CLOEXEC`;
- затем fd оборачивается в `os.NewFile`;
- после этого выполняется `TUNSETIFF`.

Это убрало постоянные `not pollable` ошибки в netns stress tests.

## 4. Как работает демо-режим без TUN

Демо-режим нужен для проверки логики без root-прав.

Сервер запускается:

```sh
go run ./cmd/server -listen0 127.0.0.1:44433 -listen1 127.0.0.1:44434
```

Клиент запускается:

```sh
go run ./cmd/client \
  -server0 127.0.0.1:44433 \
  -server1 127.0.0.1:44434 \
  -count 4
```

Последовательность:

1. Сервер поднимает два QUIC listener.
2. Клиент устанавливает два QUIC connection.
3. Клиент открывает stream на каждом connection.
4. Клиент строит ICMP echo request.
5. Scheduler выбирает path 0.
6. Packet отправляется frame-ом.
7. Сервер читает frame, строит ICMP echo reply, отправляет обратно.
8. Клиент валидирует reply.
9. Следующий packet идет через path 1.
10. Цикл повторяется.

Ожидаемый результат для `-count 4`:

```text
path 0 echo reply sequence=1
path 1 echo reply sequence=2
path 0 echo reply sequence=3
path 1 echo reply sequence=4
```

## 5. Как работает client TUN + synthetic server

Этот режим проверяет, что клиентский TUN реально получает пакеты от ядра.

Сначала на клиенте создается TUN:

```sh
sudo ./scripts/setup-client.sh
```

Затем запускается обычный demo server, а клиент запускается с `-tun`:

```sh
go run ./cmd/client \
  -tun \
  -tun-name mvpvpn0 \
  -server0 127.0.0.1:44433 \
  -server1 127.0.0.1:44434
```

Когда выполняется:

```sh
ping 10.8.0.1
```

ядро смотрит route `10.8.0.1/32 dev mvpvpn0` и отправляет ICMP packet в TUN.
Клиент читает этот packet, отправляет его серверу через QUIC, сервер строит
ICMP echo reply, клиент пишет reply обратно в `mvpvpn0`, и `ping` видит ответ.

## 6. Как работает full TUN-to-TUN

Full TUN-to-TUN - основной режим, похожий на VPN.

На сервере:

```sh
sudo ./scripts/setup-server.sh
go run ./cmd/server \
  -tun \
  -tun-name mvpvpns0 \
  -listen0 127.0.0.1:44433 \
  -listen1 127.0.0.1:44434
```

На клиенте:

```sh
sudo ./scripts/setup-client.sh
go run ./cmd/client \
  -tun \
  -tun-name mvpvpn0 \
  -server0 127.0.0.1:44433 \
  -server1 127.0.0.1:44434
```

Packet flow:

1. `ping 10.8.0.1` создает ICMP echo request.
2. Linux route отправляет packet в `mvpvpn0`.
3. Клиент читает packet из TUN.
4. Клиент выбирает QUIC path.
5. Клиент отправляет frame на сервер.
6. Сервер получает frame.
7. Сервер пишет raw packet в `mvpvpns0`.
8. Ядро сервера отвечает ICMP echo reply.
9. Сервер читает reply из `mvpvpns0`.
10. Сервер выбирает активный QUIC stream.
11. Сервер отправляет reply клиенту.
12. Клиент пишет reply в `mvpvpn0`.
13. `ping` получает ответ.

## 7. Конкурентная модель

В проекте активно используются goroutines.

На сервере в demo mode:

- по одной goroutine на listener;
- по одной goroutine на accepted connection;
- внутри connection принимается stream и обрабатываются frames.

На сервере в TUN mode:

- goroutine читает packets из server TUN;
- goroutine на каждый listener;
- goroutine на каждый accepted stream;
- записи в TUN защищены mutex-ом;
- отправка из TUN обратно клиенту идет round-robin по активным streams.

На клиенте в TUN mode:

- goroutine читает packets из client TUN;
- goroutine на каждый active path читает frames из QUIC и пишет packets в TUN;
- goroutine на каждый configured path следит за reconnect;
- `clientPathSet` защищен mutex-ом;
- записи в TUN защищены mutex-ом, потому что несколько receiver goroutines
  могут одновременно получить packets.

Завершение управляется через `context.Context`. При cancel закрываются TUN fd и
QUIC listeners, чтобы разблокировать pending read/accept операции.

Отдельно добавлен `internal/buildinfo`, который позволяет вывести версию
клиента или сервера через `-version`. Значение версии можно заменить при сборке
через Go `-ldflags`.

## 8. Обработка ошибок и отказоустойчивость

Реализованы следующие сценарии:

- если frame пустой, он отклоняется;
- если frame больше `65535`, он отклоняется;
- если IPv4 packet malformed, demo server считает его drop;
- если ICMP packet не echo request, demo server считает его drop;
- если QUIC stream ломается, path удаляется;
- если path только что ломался, он получает короткий cooldown;
- если один path сломан, packet отправляется через следующий активный path;
- если все paths недоступны, TUN packets drop-аются до reconnect;
- если path отсутствует, reconnect goroutine пытается восстановить его;
- если TUN fd закрыт из-за cancel, read loop завершается штатно;
- если возникает `not pollable`, выполняется bounded retry, но основная
  причина исправлена на уровне открытия TUN fd.

## 9. Что тестировалось

Автоматические unit tests покрывают:

- IPv4 parsing;
- IPv4 packet building;
- ICMP echo request/reply;
- checksum;
- frame read/write;
- frame validation;
- round-robin scheduler;
- TLS config;
- mTLS socket-level QUIC smoke;
- packet policy allow/drop;
- TUN device name normalization;
- env parsing;
- stats counters и JSON-format;
- client TUN send loop;
- client path failover;
- client path health cooldown;
- all-paths-down drops;
- reconnect backoff;
- stream graceful close detection;
- server TUN round-robin;
- server TUN forwarding;
- retry behavior for `not pollable`.

Скриптовые проверки:

- `scripts/check-tun-scripts.sh` проверяет shell syntax и dry-run helper-ов;
- `scripts/check-operational-examples.sh` проверяет env files и systemd unit
  structure.

Регулярные команды проверки:

```sh
go test ./...
go vet ./...
./scripts/check-tun-scripts.sh
./scripts/check-operational-examples.sh
```

Также выполнялись усиленные проверки:

```sh
go test -race ./...
go test -shuffle=on -count=20 ./internal/quictransport ./internal/packet ./internal/stats
go test -count=50 ./...
go test -shuffle=on -count=50 ./...
go test -race -count=10 ./...
go test -count=250 ./internal/quictransport ./internal/packet ./internal/stats
```

Интеграционные проверки включали:

- live dual-path smoke без TUN;
- single-path path0 smoke;
- single-path path1 smoke;
- env-driven smoke;
- trusted TLS smoke;
- negative CLI/config cases;
- root TUN tests;
- network namespace full TUN-to-TUN tests;
- failover через `iptables`;
- reconnect после снятия `iptables DROP`.
- MTU-проверку через `scripts/integration-mtu.sh`;
- soak-проверку full TUN-to-TUN через `scripts/integration-soak.sh`;
- CI workflow `.github/workflows/ci.yml`.

## 10. Результаты stress testing

После исправления TUN opening были выполнены тяжелые проверки:

- полный non-root heavy прогон прошел успешно;
- root/netns heavy прогон прошел успешно;
- root/netns heavy прогон был повторен и снова прошел успешно.

Root/netns сценарий включал:

- 3 цикла default namespace failover;
- 3 цикла full TUN-to-TUN через network namespaces;
- в каждом netns-цикле выполнялся `ping -c 20 10.8.0.1`;
- итог каждого netns ping: `20 packets transmitted, 20 received, 0% packet loss`;
- после фикса в логах не осталось `not pollable`.

Failover-сценарий:

1. клиент подключается к path 0 и path 1;
2. через `iptables` блокируется UDP-порт path 1;
3. path 1 перестает работать;
4. path 0 продолжает передавать traffic;
5. блокировка снимается;
6. reconnect loop восстанавливает path 1;
7. снова становится активно 2 пути.

## 11. Ограничения проекта

Проект специально оставлен MVP, поэтому у него есть ограничения:

- реальный TUN mode поддержан только на Linux;
- production authentication ограничена mTLS и требует нормального управления
  сертификатами;
- demo TLS mode отключает verify при отсутствии `-ca-cert`;
- нет automatic NAT management;
- packet policy минимальная и CIDR-based, это не полноценный firewall;
- нет маршрутизации сложных подсетей из коробки;
- нет latency-based path quality scoring;
- нет congestion coordination между путями;
- нет отдельной packet retransmission/reordering логики поверх QUIC;
- серверный demo mode отвечает только на ICMP echo request;
- full VPN deployment требует аккуратной настройки маршрутов, firewall и прав.

## 12. Практическая ценность проекта

Несмотря на MVP-статус, проект демонстрирует важные инженерные темы:

- построение сетевого прототипа на Go;
- работу с raw IPv4 packets;
- использование QUIC как транспорта;
- построение собственного минимального протокола framing;
- работу с Linux TUN;
- multipath packet scheduling;
- обработку отказов сетевых путей;
- reconnect с exponential backoff;
- mutual TLS и минимальную packet policy;
- JSON-метрики и CLI version output;
- GitHub Actions CI;
- системное тестирование с network namespaces;
- подготовку приложения к запуску как systemd service;
- отличие demo security от trusted TLS mode.

Для курсовой этот проект можно описывать как экспериментальную реализацию
легкого QUIC VPN-туннеля с поддержкой двух транспортных путей и базовой
отказоустойчивостью.

## 13. Краткая формулировка результата

В результате был разработан прототип `mvp-vpn-lite` - легкий VPN-like туннель
на Go, передающий IPv4-пакеты через QUIC. Реализованы два режима работы:
синтетический ICMP demo mode и реальный Linux TUN-to-TUN mode. Добавлена
поддержка двух QUIC-путей, round-robin распределение пакетов, удаление
сломанных путей, reconnect с bounded backoff, TLS demo/trusted modes,
mutual TLS, packet policy, text/JSON статистика, helper-скрипты,
env-конфигурация, systemd-примеры и подробная документация. Проект покрыт
unit, script, smoke, race, stress, CI и root/netns
integration проверками. В ходе stress testing была найдена и исправлена
нестабильность Linux TUN fd, связанная с `read /dev/net/tun: not pollable`.
