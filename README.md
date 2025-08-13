# MCP Operator для Kubernetes

MCP (Model Context Protocol) Operator - это enterprise-уровневый Kubernetes-оператор, который обеспечивает полнофункциональное развертывание, управление и мониторинг MCP серверов в кластере Kubernetes.

## Описание

Оператор предоставляет декларативное управление MCP серверами через Custom Resource Definitions (CRD), автоматически создавая и управляя всеми необходимыми Kubernetes ресурсами. Включает продвинутые возможности корпоративного уровня: автомасштабирование, резервное копирование, интеграцию с service mesh, многопользовательские конфигурации и высокопроизводительное кеширование.

## Возможности

### Основные функции
- ✅ **Декларативное управление** MCP серверами через Custom Resource Definition (CRD)
- ✅ **Автоматическое создание** Deployment, Service и других объектов для каждого MCP сервера
- ✅ **Поддержка различных типов сервисов** (ClusterIP, NodePort, LoadBalancer)
- ✅ **Гибкое конфигурирование** ресурсов, переменных окружения и аргументов
- ✅ **Продвинутое размещение подов** с Node Selector, Tolerations и Affinity
- ✅ **Комплексный статус-мониторинг** состояния MCP серверов
- ✅ **Ручное и автоматическое масштабирование** (HPA/VPA)

### Автомасштабирование и производительность
- ✅ **Horizontal Pod Autoscaler (HPA)** с поддержкой CPU, памяти и custom метрик
- ✅ **Vertical Pod Autoscaler (VPA)** для оптимизации ресурсов
- ✅ **Настраиваемые политики масштабирования** и поведение HPA
- ✅ **High-performance кеширование** с индексированием для быстрого поиска
- ✅ **Intelligent indexing** для эффективных запросов к Kubernetes API
- ✅ **Автоматическая очистка кеша** и мониторинг производительности

### Резервное копирование и восстановление
- ✅ **MCPServerBackup** - полнофункциональная система резервного копирования
- ✅ **Множественные storage backends**: S3, Google Cloud Storage, Azure Blob Storage, локальное хранение
- ✅ **Гибкие политики хранения** с автоматическим удалением старых бэкапов
- ✅ **Инкрементальные и полные резервные копии**
- ✅ **Шифрование и сжатие** резервных копий
- ✅ **Восстановление из точки времени** с проверкой целостности данных

### Интеграция с Service Mesh
- ✅ **Istio integration** с автоматической настройкой VirtualService и DestinationRule
- ✅ **Traffic management** с load balancing, retry policies, и circuit breakers
- ✅ **mTLS configuration** и продвинутые security policies
- ✅ **Connection pooling** и HTTP/TCP настройки

### Безопасность и стабильность
- ✅ **Финализаторы** для корректной очистки ресурсов при удалении
- ✅ **Graceful Shutdown** с ожиданием завершения подов
- ✅ **Security Context** с безопасными настройками по умолчанию
- ✅ **Pod Security Standards** compliance
- ✅ **Контроль ресурсов** с лимитами по умолчанию

### Наблюдаемость и мониторинг
- ✅ **Структурированное логирование** с correlation ID для отслеживания операций
- ✅ **Prometheus метрики** для мониторинга производительности оператора
- ✅ **Health Checks** и **Readiness Probes** для контейнеров MCP серверов
- ✅ **Детальная статусная информация** о состоянии ресурсов
- ✅ **Метрики операций** (создание, обновление, удаление)

### Multi-tenancy и изоляция
- ✅ **TenancySpec** - полноценная поддержка многопользовательских конфигураций
- ✅ **Resource Quotas** для ограничения ресурсов по tenant
- ✅ **Network Policies** с автоматической настройкой изоляции сети
- ✅ **Namespace isolation** с гибкими правилами доступа
- ✅ **RBAC integration** для контроля доступа на уровне tenant

### Управление томами и конфигурациями
- ✅ **Гибкое управление томами** с поддержкой ConfigMap, Secret, PVC, HostPath
- ✅ **Projected volumes** для комплексного монтирования конфигураций
- ✅ **ServiceAccount token projection** для безопасной аутентификации
- ✅ **Автоматическое управление конфигурациями** из различных источников
- ✅ **Dynamic configuration reloading** при изменении ConfigMap и Secret

### Deployment strategies и rollout management
- ✅ **Настраиваемые стратегии развертывания** (Recreate, RollingUpdate)
- ✅ **Fine-grained rolling update control** с MaxUnavailable и MaxSurge
- ✅ **Deployment lifecycle management** с полным контролем процесса

### Интеграция с реестром и GitHub
- ✅ **MCPRegistry** - автоматическая загрузка спецификаций серверов из MCP Registry
- ✅ **Intelligent GitHub retry** с экспоненциальной задержкой и обработкой rate limits
- ✅ **GitHub API rate limit handling** с автоматическим ожиданием до 1 часа
- ✅ **Network error resilience** с separate retry logic для сетевых сбоев
- ✅ **Автоматическое обогащение** MCPServer ресурсов данными из реестра
- ✅ **Хранение в Kubernetes** - данные реестра сохраняются в etcd
- ✅ **Статус-трекинг** операций загрузки из реестра с детальными условиями
- ✅ **Синхронизация** конфигураций с внешними источниками
- ✅ **Метрики реестра** для отслеживания операций загрузки

### Управление зависимостями
- ✅ **Renovate Bot** для автоматического обновления зависимостей
- ✅ **Еженедельные обновления** Go модулей, GitHub Actions и Docker образов
- ✅ **Группировка обновлений** по типам зависимостей
- ✅ **Автоматические уведомления** о уязвимостях безопасности

### Webhook и автоматические значения по умолчанию
- ✅ **Мутирующий webhook** автоматически применяет разумные значения по умолчанию
- ✅ **Валидирующий webhook** проверяет корректность конфигурации
- ✅ **Автоматические значения по умолчанию**:
  - `port`: 8080 (если не указан)
  - `replicas`: 1 (если не указано)
  - `serviceType`: ClusterIP (если не указан)
  - `resources.requests`: cpu: 100m, memory: 128Mi (если не указаны)
  - `resources.limits`: cpu: 500m, memory: 512Mi (если не указаны)
  - `securityContext`: runAsNonRoot: true, runAsUser: 1000 (если не указан)

## Быстрый старт

### Предварительные требования

- Kubernetes 1.19+
- kubectl
- Helm 3.2.0+ (для установки через Helm)
- Go 1.21+ (для разработки)

### Установка

#### Установка через Helm (Рекомендуется)

##### Метод 1: Из Helm репозитория (Рекомендуется)

1. Добавьте Helm репозиторий:
```bash
helm repo add mcp-operator https://fantasynitrogen.github.io/mcp_operator/
helm repo update
```

2. Установите оператор:
```bash
helm install mcp-operator mcp-operator/mcp-operator
```

3. Проверьте статус установки:
```bash
kubectl get pods -l app.kubernetes.io/name=mcp-operator
```

##### Метод 2: Из исходного кода

1. Склонируйте репозиторий:
```bash
git clone https://github.com/FantasyNitroGEN/mcp_operator.git
cd mcp-operator
```

2. Установите оператор с помощью Helm:
```bash
helm install mcp-operator ./helm/mcp-operator
```

3. Проверьте статус установки:
```bash
kubectl get pods -l app.kubernetes.io/name=mcp-operator
```

#### Установка для разработки

1. Установите CRD:
```bash
make install
```

2. Запустите оператор:
```bash
make run
```

## MCP CLI

MCP Operator поставляется с полнофункциональным CLI инструментом, разработанным по аналогии с kubectl, для управления MCP серверами без необходимости создавать YAML манифесты вручную. CLI предоставляет все необходимые возможности для работы с оператором через командную строку с консистентными флагами и форматами вывода.

### Ключевые возможности CLI
- ✅ **kubectl-подобный интерфейс** с консистентными флагами и поведением
- ✅ **Унифицированные флаги вывода** (`--output/-o`) для всех команд
- ✅ **Kubernetes session флаги** (`--namespace/-n`, `--kubeconfig`, `--all-namespaces/-A`)
- ✅ **Операционные флаги управления** (`--dry-run`, `--wait`, `--timeout`)
- ✅ **Валидация флагов** с понятными сообщениями об ошибках
- ✅ **Поддержка форматов** table, json, yaml для всех команд
- ✅ **Интеллектуальная обработка ошибок** и helpful help messages

### Установка CLI

#### Сборка из исходного кода

```bash
# Склонируйте репозиторий (если еще не сделали)
git clone https://github.com/FantasyNitroGEN/mcp_operator.git
cd mcp-operator

# Соберите CLI
go build -o bin/mcp ./cmd/mcp

# Переместите в PATH (опционально)
sudo mv bin/mcp /usr/local/bin/
```

### Использование CLI

#### Просмотр доступных серверов в реестре

```bash
# Список всех доступных MCP серверов
mcp registry list

# Поиск серверов по ключевому слову
mcp registry search filesystem

# Просмотр детальной спецификации сервера
mcp registry inspect filesystem

# Просмотр спецификации в JSON формате
mcp registry inspect filesystem --format json
```

Команда `inspect` позволяет просмотреть полную спецификацию MCP сервера из реестра, включая описание, версию, автора, runtime конфигурацию, переменные окружения, возможности и схему конфигурации. Пример вывода:

```yaml
name: filesystem
version: ""
description: The MCP server is allowed to access these paths
repository: ""
license: ""
author: ""
homepage: ""
keywords: []
runtime:
    type: ""
    image: ""
    command: []
    args: []
    env: {}
config:
    schema: []
    required: []
    properties: {}
capabilities: []
environment: {}
templatedigest: 1855e7e6c00a6984093b1a9ea047c5c2f00af160c5921a5db2460bc0a1837565
```

```bash
# Список с JSON выводом
mcp registry list --format json

# Обновить кэш реестра (синхронизация с удаленным реестром)
mcp registry refresh

# Обновить конкретный реестр по имени
mcp registry refresh --name myregistry

# Обновить реестр в определенном namespace
mcp registry refresh --namespace mcp-system
```

#### Развертывание серверов

```bash
# Развернуть сервер из реестра
mcp deploy filesystem-server

# Развернуть в определенный namespace
mcp deploy filesystem-server --namespace mcp-servers

# Развернуть с несколькими репликами
mcp deploy filesystem-server --replicas 3

# Предварительный просмотр (dry-run)
mcp deploy filesystem-server --dry-run

# Развернуть и дождаться готовности
mcp deploy filesystem-server --wait --wait-timeout 5m
```

#### Проверка статуса серверов

```bash
# Получить статус сервера
mcp status filesystem-server

# Получить статус из определенного namespace
mcp status filesystem-server --namespace mcp-servers

# Получить статус в JSON формате
mcp status filesystem-server --output json

# Получить статус в YAML формате
mcp status filesystem-server --output yaml

# Отслеживать изменения статуса в реальном времени
mcp status filesystem-server --watch
```

#### Просмотр логов серверов

```bash
# Просмотреть логи сервера
mcp server logs filesystem-server

# Просмотреть логи из определенного namespace
mcp server logs filesystem-server --namespace mcp-servers

# Следить за логами в реальном времени (как kubectl logs -f)
mcp server logs filesystem-server --follow

# Показать последние 50 строк логов
mcp server logs filesystem-server --tail 50

# Указать конкретный контейнер (если в поде несколько контейнеров)
mcp server logs filesystem-server --container mcp-server

# Комбинированное использование флагов
mcp server logs filesystem-server --namespace mcp-servers --follow --tail 100
```

#### Удаление серверов

```bash
# Удалить сервер
mcp delete filesystem-server

# Удалить из определенного namespace
mcp delete filesystem-server --namespace mcp-servers

# Удалить и дождаться полного удаления всех ресурсов
mcp delete filesystem-server --wait

# Удалить с настраиваемым таймаутом ожидания
mcp delete filesystem-server --wait --wait-timeout 10m
```

#### Дополнительные команды

```bash
# Показать версию CLI
mcp version

# Помощь по командам
mcp --help
mcp registry --help
mcp registry refresh --help
mcp deploy --help
mcp status --help
mcp server --help
mcp server logs --help
mcp delete --help
```

## MCPServerBackup - Резервное копирование

MCP Operator включает полнофункциональную систему резервного копирования для MCP серверов через ресурс `MCPServerBackup`.

### Возможности резервного копирования

#### Типы резервных копий
- **Full Backup**: Полная резервная копия всех данных сервера
- **Incremental Backup**: Инкрементальные копии для экономии места
- **Configuration Backup**: Резервное копирование только конфигураций

#### Поддерживаемые storage backends
- **S3 Compatible Storage**: AWS S3, MinIO, и другие S3-совместимые решения
- **Google Cloud Storage**: Нативная интеграция с GCS
- **Azure Blob Storage**: Полная поддержка Azure Storage
- **Local Storage**: PVC, HostPath, EmptyDir для локального хранения

#### Политики хранения
- **Автоматическое удаление старых бэкапов** по времени или количеству
- **Гибкие retention policies** с поддержкой различных правил
- **Lifecycle management** для оптимизации затрат на хранение

### Пример конфигурации MCPServerBackup

```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServerBackup
metadata:
  name: filesystem-server-backup
spec:
  # Ссылка на MCP сервер для резервного копирования
  serverRef:
    name: filesystem-server
    namespace: default
  
  # Тип резервной копии
  backupType: Full
  
  # Конфигурация хранилища
  storage:
    type: S3
    s3:
      bucket: mcp-backups
      region: us-west-2
      endpoint: s3.amazonaws.com
      credentials:
        accessKeyId:
          secretKeyRef:
            name: s3-credentials
            key: access-key-id
        secretAccessKey:
          secretKeyRef:
            name: s3-credentials  
            key: secret-access-key
  
  # Политика хранения
  retention:
    keepDaily: 7      # Хранить ежедневные бэкапы 7 дней
    keepWeekly: 4     # Хранить еженедельные бэкапы 4 недели  
    keepMonthly: 12   # Хранить ежемесячные бэкапы 12 месяцев
    keepYearly: 5     # Хранить годовые бэкапы 5 лет
  
  # Расписание (поддерживается cron формат)
  schedule: "0 2 * * *"  # Каждый день в 2:00 AM
  
  # Дополнительные опции
  compression: gzip
  encryption: true
```

### Управление резервными копиями

```bash
# Создать резервную копию
kubectl apply -f mcpserver-backup.yaml

# Просмотреть статус резервного копирования
kubectl get mcpserverbackups

# Детальная информация о резервной копии
kubectl describe mcpserverbackup filesystem-server-backup

# Просмотреть логи процесса резервного копирования
kubectl logs -l app.kubernetes.io/component=backup-controller
```

### Развертывание MCP сервера

Создайте файл с описанием MCP сервера:

```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: filesystem-server
spec:
  image: "mcp-server-filesystem"
  tag: "latest"
  replicas: 1
  port: 8080
  command: ["python", "-m", "mcp_server_filesystem"]
  args: ["--allowed-dirs", "/app/data"]
  env:
    - name: MCP_SERVER_NAME
      value: "filesystem-server"
  resources:
    requests:
      memory: "64Mi"
      cpu: "50m"
    limits:
      memory: "128Mi"
      cpu: "100m"
```

Примените конфигурацию:
```bash
kubectl apply --server-side -f mcpserver.yaml
```

#### Развертывание с автоматическим обогащением из реестра

Новая возможность! Теперь вы можете создать MCPServer, указав только имя сервера из реестра, и оператор автоматически заполнит все необходимые поля:

```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: filesystem-server
spec:
  registry:
    name: filesystem-server  # Имя сервера из MCP Registry
  runtime:
    type: python  # Тип среды выполнения (будет обогащен из реестра)
  # Оператор автоматически заполнит:
  # - registry.version, registry.description, registry.repository и т.д.
  # - runtime.image, runtime.command, runtime.args
  # - runtime.env (переменные окружения)
```

Примените конфигурацию:
```bash
kubectl apply --server-side -f mcpserver-registry.yaml
```

Оператор автоматически:
1. Загрузит спецификацию сервера из MCP Registry
2. Обогатит ресурс данными из реестра
3. Установит условие `RegistryFetched=True` в статусе
4. Развернет сервер с полученными параметрами

### Проверка статуса

#### Использование MCP CLI (рекомендуется)

Для удобной проверки статуса используйте команду `mcp status`:

```bash
# Получить статус сервера в удобном формате
mcp status filesystem-server

# Пример вывода:
# Name: filesystem-server
# Namespace: default
# Phase: Running
# Replicas: 1/1
# Endpoint: filesystem-server.default.svc.cluster.local:8080
# Age: 5m
# Registry: filesystem-server
# 
# Conditions:
#   ✓ Ready: True
#   ✓ Available: True
#   ✓ RegistryFetched: True - Registry data successfully fetched
```

#### Использование kubectl

Альтернативно, можно использовать kubectl:

```bash
kubectl get mcpservers

NAME                IMAGE                    TAG      REPLICAS   READY   PHASE     AGE
filesystem-server   mcp-server-filesystem    latest   1          1       Running   2m
```

Детальная информация:
```bash
kubectl describe mcpserver filesystem-server
```

Проверка статуса загрузки из реестра:
```bash
# Проверка статуса загрузки из реестра
kubectl get mcpserver filesystem-server -o jsonpath='{.status.conditions[?(@.type=="RegistryFetched")].status}'

# Просмотр сообщения о статусе реестра
kubectl get mcpserver filesystem-server -o jsonpath='{.status.conditions[?(@.type=="RegistryFetched")].message}'

# Просмотр всех условий статуса
kubectl get mcpserver filesystem-server -o jsonpath='{.status.conditions[*].type}'
```

## Конфигурация MCPServer

### Основные параметры

| Поле | Тип | Описание | По умолчанию |
|------|-----|----------|-------------|
| `image` | string | Docker образ MCP сервера | обязательное |
| `tag` | string | Тег образа | `latest` |
| `replicas` | int32 | Количество реплик | `1` |
| `port` | int32 | Порт сервера | `8080` |
| `serviceType` | string | Тип сервиса (ClusterIP/NodePort/LoadBalancer) | `ClusterIP` |
| `command` | []string | Команда для запуска | |
| `args` | []string | Аргументы команды | |
| `env` | []EnvVar | Переменные окружения | |
| `resources` | ResourceRequirements | Ресурсы контейнера | |

### Размещение подов

```yaml
spec:
  # Размещение на определенных узлах
  nodeSelector:
    disktype: ssd

  # Tolerations для работы на узлах с taint
  tolerations:
    - key: "mcp-servers"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"

  # Правила размещения
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
            - key: app.kubernetes.io/name
              operator: In
              values:
              - mcpserver
          topologyKey: kubernetes.io/hostname
```

## Примеры MCP серверов

### Файловый сервер
```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: filesystem-server
spec:
  image: "python"
  tag: "3.11-slim"
  command: ["python", "-m", "mcp_server_filesystem"]
  args: ["--allowed-dirs", "/app/data"]
  env:
    - name: PYTHONPATH
      value: "/app"
```

### Git сервер
```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: git-server
spec:
  image: "mcp-server-git"
  tag: "latest"
  command: ["mcp-server-git"]
  env:
    - name: GIT_REPOS_PATH
      value: "/repos"
```

### PostgreSQL сервер
```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: postgres-server
spec:
  image: "mcp-server-postgres"
  tag: "latest"
  env:
    - name: DATABASE_URL
      valueFrom:
        secretKeyRef:
          name: postgres-secret
          key: database-url
```

## Разработка

### Сборка
```bash
make build
```

### Тестирование
```bash
make test
```

### Генерация кода
```bash
make generate
make manifests
```

### Сборка Docker образа
```bash
make docker-build IMG=your-registry/mcp-operator:tag
```

### Развертывание в кластер
```bash
make deploy IMG=your-registry/mcp-operator:tag
```

## Архитектура

MCP Operator представляет собой enterprise-уровневое решение с модульной архитектурой:

### Основные компоненты

#### Custom Resource Definitions (CRD)
- **MCPServer**: Определяет спецификацию MCP сервера с поддержкой автомасштабирования, multi-tenancy, и интеграции с service mesh
- **MCPRegistry**: Управляет реестрами MCP серверов с поддержкой GitHub и других источников
- **MCPServerBackup**: Обеспечивает резервное копирование с множественными storage backends

#### Контроллеры
- **MCPServer Controller**: Управляет жизненным циклом MCP серверов
- **MCPRegistry Controller**: Синхронизирует данные из внешних реестров с intelligent retry логикой
- **MCPServerBackup Controller**: Управляет процессами резервного копирования и восстановления

#### Сервисы и компоненты
- **Cache Service**: High-performance кеширование с TTL и автоматической инвалидацией
- **Registry Service**: Интеграция с GitHub API и другими источниками реестров
- **GitHub Retry Service**: Intelligent retry механизмы с обработкой rate limits
- **Metrics Service**: Prometheus метрики для мониторинга производительности
- **Webhook Service**: Валидация и мутация ресурсов с автоматическими значениями по умолчанию

### Создаваемые ресурсы

#### Для MCPServer
- **Deployment**: Управляет подами с поддержкой HPA/VPA и custom deployment strategies
- **Service**: Обеспечивает сетевой доступ с поддержкой различных типов сервисов
- **ConfigMap/Secret**: Хранит конфигурации с поддержкой projected volumes
- **HorizontalPodAutoscaler/VerticalPodAutoscaler**: Автоматическое масштабирование (опционально)
- **NetworkPolicy**: Изоляция сети для multi-tenant сценариев (опционально)
- **ServiceMonitor**: Интеграция с Prometheus для мониторинга (опционально)
- **VirtualService/DestinationRule**: Istio service mesh интеграция (опционально)

#### Для MCPRegistry
- **ConfigMap**: Кеширование данных реестра для быстрого доступа
- **Secret**: Хранение credentials для доступа к приватным реестрам

#### Для MCPServerBackup
- **Job/CronJob**: Выполнение процессов резервного копирования
- **PVC**: Временное хранение для backup операций
- **Secret**: Credentials для доступа к внешним storage системам

### Архитектурные паттерны

- **Event-driven architecture**: Reactive обработка изменений ресурсов
- **Circuit breaker pattern**: Защита от каскадных сбоев при работе с внешними API
- **Cache-aside pattern**: Эффективное кеширование с автоматической инвалидацией
- **Retry with exponential backoff**: Resilient интеграция с внешними сервисами
- **Multi-tenancy**: Изоляция ресурсов и конфигураций между tenant'ами

## Конфигурация оператора

MCP Operator предоставляет множество настраиваемых параметров для оптимизации работы в различных средах.

### Параметры командной строки

#### Основные параметры
```bash
--metrics-bind-address=:8080        # Адрес для метрик Prometheus
--leader-elect=true                  # Включить leader election для HA
--health-probe-bind-address=:8081    # Адрес для health probes
--kubeconfig=/path/to/kubeconfig     # Путь к kubeconfig файлу
--namespace=mcp-operator-system      # Namespace для работы оператора
```

#### GitHub Retry конфигурация
```bash
--github-max-retries=5                      # Общие попытки retry (по умолчанию: 5)
--github-initial-delay=1s                   # Начальная задержка для общих ошибок
--github-max-delay=5m                       # Максимальная задержка для общих ошибок
--github-rate-limit-max-retries=3           # Попытки retry для rate limit (по умолчанию: 3)
--github-rate-limit-base-delay=15m          # Базовая задержка для rate limit (по умолчанию: 15m)
--github-rate-limit-max-delay=1h            # Максимальная задержка для rate limit (по умолчанию: 1h)
--github-network-max-retries=5              # Попытки retry для сетевых ошибок (по умолчанию: 5)
--github-network-base-delay=2s              # Базовая задержка для сетевых ошибок
--github-network-max-delay=2m               # Максимальная задержка для сетевых ошибок
```

#### Кеширование и производительность
```bash
--cache-registry-ttl=10m            # TTL для кеша реестров (по умолчанию: 10 минут)
--cache-server-ttl=5m               # TTL для кеша серверов (по умолчанию: 5 минут)
--cache-cleanup-interval=30m        # Интервал очистки истёкших записей (по умолчанию: 30 минут)
--indexing-enabled=true             # Включить индексирование для оптимизации запросов
--concurrent-reconciles=5           # Количество одновременных reconcile операций
```

#### Webhook конфигурация
```bash
--webhook-port=9443                 # Порт для webhook сервера
--webhook-cert-dir=/tmp/k8s-webhook-server/serving-certs  # Директория для сертификатов
--webhook-tls-min-version=1.2       # Минимальная версия TLS
--webhook-cipher-suites=...         # Разрешённые cipher suites
```

### Helm Values конфигурация

#### Основные настройки
```yaml
# values.yaml
replicaCount: 1

image:
  repository: mcp-operator
  tag: latest
  pullPolicy: IfNotPresent

# Ресурсы для оператора
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi

# Node selector для размещения оператора
nodeSelector: {}

# Tolerations для работы на специальных узлах
tolerations: []

# Affinity rules
affinity: {}
```

#### GitHub интеграция
```yaml
github:
  retry:
    enabled: true
    maxRetries: 5
    initialDelay: "1s"
    maxDelay: "5m"
    rateLimitMaxRetries: 3
    rateLimitBaseDelay: "15m"
    rateLimitMaxDelay: "1h"
    networkMaxRetries: 5
    networkBaseDelay: "2s"
    networkMaxDelay: "2m"
```

#### Кеширование
```yaml
cache:
  enabled: true
  registryTTL: "10m"
  serverTTL: "5m"
  cleanupInterval: "30m"
  
indexing:
  enabled: true
```

#### Метрики и мониторинг
```yaml
metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: true
    interval: 30s
    labels: {}

prometheus:
  enabled: true
  rules:
    enabled: true
    
grafana:
  dashboards:
    enabled: true
```

#### Webhook настройки
```yaml
webhook:
  enabled: true
  port: 9443
  failurePolicy: Fail
  certManager:
    enabled: true
    issuer: selfsigned-issuer
```

#### Резервное копирование
```yaml
backup:
  enabled: true
  defaultStorageClass: standard
  retentionPolicies:
    daily: 7
    weekly: 4
    monthly: 12
    yearly: 5
  
  # Настройки для различных storage backends
  s3:
    region: us-west-2
    endpoint: s3.amazonaws.com
    
  gcs:
    projectId: my-project
    
  azure:
    accountName: mystorageaccount
```

### Переменные окружения

```bash
# Logging уровень
export LOG_LEVEL=info              # debug, info, warn, error

# Включить structured logging
export STRUCTURED_LOGGING=true

# Корреляция ID для трассировки
export ENABLE_CORRELATION_ID=true

# Профилирование производительности
export ENABLE_PPROF=false
export PPROF_ADDR=:6060

# Лимиты для операций
export MAX_CONCURRENT_RECONCILES=10
export RECONCILE_TIMEOUT=30m

# Feature flags
export ENABLE_HPA_SUPPORT=true
export ENABLE_VPA_SUPPORT=true
export ENABLE_ISTIO_INTEGRATION=true
export ENABLE_MULTI_TENANCY=true
export ENABLE_BACKUP_CONTROLLER=true
```

### Примеры конфигурации

#### Высоконагруженная среда
```bash
./manager \
  --concurrent-reconciles=10 \
  --cache-cleanup-interval=15m \
  --github-max-retries=3 \
  --github-rate-limit-max-delay=2h \
  --metrics-bind-address=:8080
```

#### Development среда
```bash
./manager \
  --log-level=debug \
  --github-network-max-retries=10 \
  --cache-registry-ttl=1m \
  --cache-server-ttl=30s \
  --enable-pprof=true
```

#### Production среда с HA
```bash
./manager \
  --leader-elect=true \
  --concurrent-reconciles=5 \
  --github-rate-limit-max-delay=1h \
  --structured-logging=true \
  --enable-correlation-id=true
```

## Лицензия

MIT License - см. [LICENSE](LICENSE) файл.

## Contributing

1. Fork проект
2. Создайте feature branch (`git checkout -b feature/amazing-feature`)
3. Commit изменения (`git commit -m 'Add amazing feature'`)
4. Push в branch (`git push origin feature/amazing-feature`)
5. Создайте Pull Request

## Roadmap

### ✅ Реализованные возможности
- [x] **Поддержка Helm charts** с полной интеграцией
- [x] **Webhook для валидации и мутации** с автоматическими значениями по умолчанию
- [x] **Comprehensive metrics и мониторинг** с Prometheus интеграцией
- [x] **Auto-scaling (HPA/VPA)** на основе нагрузки и ресурсов
- [x] **Интеграция с Service Mesh (Istio)** с VirtualService/DestinationRule
- [x] **Поддержка секретов и ConfigMaps** с projected volumes
- [x] **Backup и восстановление конфигураций** через MCPServerBackup
- [x] **MCPRegistry** для автоматической интеграции с реестрами
- [x] **High-performance кеширование** с intelligent indexing
- [x] **GitHub API retry механизмы** с rate limit handling
- [x] **Multi-tenancy support** с network policies и resource quotas
- [x] **Volume management** с множественными типами томов
- [x] **Deployment strategies** с fine-grained control
- [x] **kubectl-like CLI** с консистентными флагами
- [x] **Enterprise-level logging** со structured logging и correlation ID

### 🚧 В разработке
- [ ] **OLM (Operator Lifecycle Manager)** интеграция для Red Hat OpenShift
- [ ] **ArgoCD/Flux** GitOps интеграция
- [ ] **Cross-cluster deployment** для multi-cluster сценариев
- [ ] **Advanced RBAC** с fine-grained permissions

### 📋 Запланированные улучшения
- [ ] **Machine Learning based auto-scaling** с predictive scaling
- [ ] **Service mesh routing** на основе AI/ML workload patterns
- [ ] **Advanced backup strategies** с cross-region replication
- [ ] **Chaos engineering** интеграция для resilience testing
- [ ] **Advanced observability** с distributed tracing
- [ ] **Policy as Code** с OPA (Open Policy Agent) интеграция
- [ ] **Cost optimization** с cloud provider интеграцией
- [ ] **Advanced security scanning** с vulnerability assessment
