package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/your-org/mcp-operator/pkg/registry"
	"github.com/your-org/mcp-operator/pkg/sync"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: sync [list|download|sources|clone-direct] [server-name]")
		os.Exit(1)
	}

	command := os.Args[1]
	ctx := context.Background()

	client := registry.NewClient()
	syncer := sync.NewSyncer(client, "servers")

	switch command {
	case "list":
		err := listServers(ctx, client)
		if err != nil {
			log.Fatal(err)
		}
	case "download":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync download <server-name>")
			os.Exit(1)
		}
		serverName := os.Args[2]
		err := downloadServer(ctx, syncer, serverName)
		if err != nil {
			log.Fatal(err)
		}
	case "sources":
		if len(os.Args) < 3 {
			fmt.Println("Usage: sync sources <server-name>")
			os.Exit(1)
		}
		serverName := os.Args[2]
		err := downloadSources(ctx, syncer, serverName)
		if err != nil {
			log.Fatal(err)
		}
	case "sync-all":
		err := syncAllServers(ctx, syncer)
		if err != nil {
			log.Fatal(err)
		}
	case "clone-direct":
		err := cloneRegistry(ctx)
		if err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println("Unknown command:", command)
		fmt.Println("Available commands: list, download, sources, sync-all, clone-direct")
		os.Exit(1)
	}
}

func listServers(ctx context.Context, client *registry.Client) error {
	// Сначала проверим наличие локального реестра
	registryPath := "mcp-registry"
	if client.HasLocalRegistry(registryPath) {
		fmt.Println("🔍 Используем локальный MCP Registry...")

		servers, err := client.ListServersLocal(ctx, registryPath)
		if err != nil {
			fmt.Printf("❌ Ошибка чтения локального реестра: %v\n", err)
			return fmt.Errorf("failed to list local servers: %w", err)
		}

		fmt.Printf("✅ Найдено %d MCP серверов в локальном реестре:\n\n", len(servers))

		// Показываем первые 20 серверов для краткости
		displayCount := len(servers)
		if displayCount > 20 {
			displayCount = 20
		}

		for i, server := range servers[:displayCount] {
			fmt.Printf("%d. 📦 %s\n", i+1, server.Name)
			fmt.Printf("   Path: %s\n", server.Path)
			fmt.Printf("   Updated: %s\n", server.UpdatedAt.Format("2006-01-02 15:04:05"))
			fmt.Println()
		}

		if len(servers) > 20 {
			fmt.Printf("... и еще %d серверов\n", len(servers)-20)
			fmt.Println("\nДля просмотра полного списка используйте:")
			fmt.Println("  make registry-list-local")
		}

		return nil
	}

	// Если локального реестра нет, пробуем GitHub API
	fmt.Println("🔍 Проверяем доступ к Docker MCP Registry...")

	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println("⚠️ GITHUB_TOKEN не установлен - возможны ограничения GitHub API")
		fmt.Println("   Получите токен: https://github.com/settings/tokens")
		fmt.Println("   Экспортируйте: export GITHUB_TOKEN=your_token")
		fmt.Println()
	}

	servers, err := client.ListServers(ctx)
	if err != nil {
		fmt.Printf("❌ Ошибка получения списка серверов: %v\n", err)
		fmt.Println("\n🔧 Попробуйте:")
		fmt.Println("1. Установите GITHUB_TOKEN:")
		fmt.Println("   export GITHUB_TOKEN=your_github_token")
		fmt.Println("2. Или используйте прямое клонирование:")
		fmt.Println("   make registry-clone-direct")
		return fmt.Errorf("failed to list servers: %w", err)
	}

	fmt.Printf("✅ Найдено %d MCP серверов в реестре:\n\n", len(servers))

	// Показываем первые 20 серверов для краткости
	displayCount := len(servers)
	if displayCount > 20 {
		displayCount = 20
	}

	for i, server := range servers[:displayCount] {
		fmt.Printf("%d. 📦 %s\n", i+1, server.Name)
		fmt.Printf("   Path: %s\n", server.Path)
		if server.HTMLURL != "" {
			fmt.Printf("   URL: %s\n", server.HTMLURL)
		}
		fmt.Println()
	}

	if len(servers) > 20 {
		fmt.Printf("... и еще %d серверов\n", len(servers)-20)
		fmt.Println("\nДля просмотра полного списка используйте:")
		fmt.Println("  make registry-list | less")
	}

	return nil
}

func downloadServer(ctx context.Context, syncer *sync.Syncer, serverName string) error {
	fmt.Printf("Загрузка описания сервера %s...\n", serverName)

	err := syncer.DownloadServerSpec(ctx, serverName)
	if err != nil {
		return fmt.Errorf("failed to download server spec: %w", err)
	}

	fmt.Printf("✅ Описание сервера %s успешно загружено в servers/yamls/\n", serverName)
	return nil
}

func downloadSources(ctx context.Context, syncer *sync.Syncer, serverName string) error {
	fmt.Printf("Загрузка исходного кода сервера %s...\n", serverName)

	err := syncer.DownloadServerSources(ctx, serverName)
	if err != nil {
		return fmt.Errorf("failed to download server sources: %w", err)
	}

	fmt.Printf("✅ Исходный код сервера %s успешно загружен в servers/sources/\n", serverName)
	return nil
}

func syncAllServers(ctx context.Context, syncer *sync.Syncer) error {
	fmt.Println("Синхронизация всех серверов из реестра...")

	err := syncer.SyncAllServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to sync all servers: %w", err)
	}

	fmt.Println("✅ Синхронизация всех серверов завершена")
	return nil
}

func cloneRegistry(ctx context.Context) error {
	fmt.Println("🔄 Клонирование Docker MCP Registry локально...")

	registryDir := "mcp-registry"
	registryURL := "https://github.com/docker/mcp-registry.git"

	// Проверяем, существует ли уже директория
	if _, err := os.Stat(registryDir); err == nil {
		fmt.Println("📁 Директория mcp-registry уже существует, обновляем...")

		// Переходим в директорию и делаем git pull
		cmd := exec.Command("git", "pull")
		cmd.Dir = registryDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to update registry: %v, output: %s", err, output)
		}
		fmt.Println("✅ Реестр успешно обновлен")
	} else {
		// Клонируем репозиторий
		fmt.Printf("📥 Клонирование из %s...\n", registryURL)
		cmd := exec.Command("git", "clone", registryURL, registryDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to clone registry: %v, output: %s", err, output)
		}
		fmt.Println("✅ Реестр успешно клонирован")
	}

	// Показываем список доступных серверов из локальной копии
	serversDir := filepath.Join(registryDir, "servers")
	if _, err := os.Stat(serversDir); err != nil {
		return fmt.Errorf("servers directory not found in cloned repository: %v", err)
	}

	entries, err := os.ReadDir(serversDir)
	if err != nil {
		return fmt.Errorf("failed to read servers directory: %v", err)
	}

	fmt.Printf("\n📦 Найдено %d MCP серверов в локальном реестре:\n\n", len(entries))

	count := 0
	for _, entry := range entries {
		if entry.IsDir() && count < 20 {
			fmt.Printf("%d. 📦 %s\n", count+1, entry.Name())

			// Проверяем наличие server.yaml
			serverFile := filepath.Join(serversDir, entry.Name(), "server.yaml")
			if _, err := os.Stat(serverFile); err == nil {
				fmt.Printf("   ✅ server.yaml найден\n")
			} else {
				fmt.Printf("   ⚠️  server.yaml не найден\n")
			}
			count++
		}
	}

	if len(entries) > 20 {
		fmt.Printf("... и еще %d серверов\n", len(entries)-20)
	}

	// Копируем server.yaml файлы в unified структуру servers/yamls/
	fmt.Println("\n🔄 Копирование server.yaml файлов в servers/yamls/...")
	client := registry.NewClient()
	syncer := sync.NewSyncer(client, "servers")

	if err := syncer.SyncFromLocalRegistry(registryDir); err != nil {
		fmt.Printf("⚠️ Ошибка копирования файлов: %v\n", err)
		fmt.Println("Файлы остались в mcp-registry/, но не скопированы в servers/yamls/")
	}

	fmt.Println("\n🎉 Теперь вы можете использовать локальные команды:")
	fmt.Println("  make registry-list-local")
	fmt.Println("  make registry-download SERVER=server-name")
	fmt.Println("  ls servers/yamls/  # Просмотр скопированных YAML файлов")

	return nil
}
