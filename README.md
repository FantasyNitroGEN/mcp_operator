# MCP Operator для Kubernetes

MCP (Model Context Protocol) Operator - это Kubernetes-оператор, который упрощает развертывание и управление MCP серверами в кластере Kubernetes.

## Описание

Оператор позволяет развертывать MCP серверы как Kubernetes-ресурсы, автоматически создавая необходимые Deployment, Service и другие объекты для каждого MCP сервера.

## Возможности

### Основные функции
- ✅ Декларативное управление MCP серверами через Custom Resource Definition (CRD)
- ✅ Автоматическое создание Deployment и Service для каждого MCP сервера
- ✅ Поддержка различных типов сервисов (ClusterIP, NodePort, LoadBalancer)
- ✅ Конфигурирование ресурсов, переменных окружения и аргументов
- ✅ Поддержка Node Selector, Tolerations и Affinity
- ✅ Статус-мониторинг состояния MCP серверов
- ✅ Масштабирование (количество реплик)

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

### Интеграция с реестром
- ✅ **Автоматическая загрузка** спецификаций серверов из MCP Registry
- ✅ **Синхронизация** конфигураций с внешними источниками
- ✅ **Метрики реестра** для отслеживания операций загрузки

### Управление зависимостями
- ✅ **Renovate Bot** для автоматического обновления зависимостей
- ✅ **Еженедельные обновления** Go модулей, GitHub Actions и Docker образов
- ✅ **Группировка обновлений** по типам зависимостей
- ✅ **Автоматические уведомления** о уязвимостях безопасности

## Быстрый старт

### Предварительные требования

- Kubernetes 1.19+
- kubectl
- Helm 3.2.0+ (для установки через Helm)
- Go 1.21+ (для разработки)

### Установка

#### Установка через Helm (Рекомендуется)

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

### Развертывание MCP сервера

Создайте файл с описанием MCP сервера:

```yaml
apiVersion: mcp.io/v1
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
kubectl apply -f mcpserver.yaml
```

### Проверка статуса

Посмотрите статус развернутых MCP серверов:

```bash
kubectl get mcpservers

NAME                IMAGE                    TAG      REPLICAS   READY   PHASE     AGE
filesystem-server   mcp-server-filesystem    latest   1          1       Running   2m
```

Детальная информация:
```bash
kubectl describe mcpserver filesystem-server
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
apiVersion: mcp.io/v1
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
apiVersion: mcp.io/v1
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
apiVersion: mcp.io/v1
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

Оператор состоит из:

- **CRD (Custom Resource Definition)**: Определяет схему ресурса `MCPServer`
- **Контроллер**: Отслеживает изменения в ресурсах `MCPServer` и создает соответствующие Kubernetes объекты
- **Webhook**: Валидация и мутация ресурсов (планируется)

### Создаваемые ресурсы

Для каждого `MCPServer` создаются:
- `Deployment`: Управляет подами с MCP сервером
- `Service`: Обеспечивает сетевой доступ к серверу
- `ConfigMap`: Хранит конфигурацию (если указана)

## Лицензия

MIT License - см. [LICENSE](LICENSE) файл.

## Contributing

1. Fork проект
2. Создайте feature branch (`git checkout -b feature/amazing-feature`)
3. Commit изменения (`git commit -m 'Add amazing feature'`)
4. Push в branch (`git push origin feature/amazing-feature`)
5. Создайте Pull Request

## Roadmap

- [ ] Поддержка Helm charts
- [ ] Webhook для валидации
- [ ] Metrics и мониторинг
- [ ] Auto-scaling на основе нагрузки
- [ ] Интеграция с Service Mesh (Istio)
- [ ] Поддержка секретов и ConfigMaps
- [ ] Backup и восстановление конфигураций
