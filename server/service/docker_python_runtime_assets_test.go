package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readPythonRuntimeInstaller(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "docker", "install-python-runtimes.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read install-python-runtimes.sh: %v", err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func TestDockerAlpineBuildUsesAliyunApkMirror(t *testing.T) {
	path := filepath.Join("..", "..", "Dockerfile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "https://mirrors.aliyun.com/alpine") {
		t.Fatal("Dockerfile 构建期必须把 apk 源换到阿里云，否则国内机器装 nodejs 会卡在官方 CDN")
	}
	if !strings.Contains(text, "mirrors.aliyun.com/alpine#g' /etc/apk/repositories \\\n    && apk add --no-cache") {
		t.Fatal("Dockerfile 必须在同一条 RUN 里先改 apk 源再 apk add，后改源救不了第一次安装")
	}
}

func TestDockerDebianBuildUsesAliyunAptMirror(t *testing.T) {
	path := filepath.Join("..", "..", "Dockerfile.debian")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Dockerfile.debian: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "COPY docker/setup-debian-apt-mirrors.sh /tmp/setup-debian-apt-mirrors.sh") {
		t.Fatal("Dockerfile.debian 必须先拷入 apt 换源脚本")
	}
	if !strings.Contains(text, "sh /tmp/setup-debian-apt-mirrors.sh \\\n    && apt-get install -y --no-install-recommends") {
		t.Fatal("Dockerfile.debian 必须在同一条 RUN 里先换源再 apt-get install，后改源救不了第一次安装")
	}
	if !strings.Contains(text, "libglib2.0-0") {
		t.Fatal("Dockerfile.debian 必须预装 Playwright Chromium 运行库，否则 headless 会报 libglib-2.0.so.0 找不到")
	}

	scriptPath := filepath.Join("..", "..", "docker", "setup-debian-apt-mirrors.sh")
	scriptData, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read setup-debian-apt-mirrors.sh: %v", err)
	}
	script := string(scriptData)
	for _, snippet := range []string{
		"http://${host}/debian",
		"mirrors.cloud.tencent.com",
		"mirrors.aliyun.com",
		"apt-cache show openssh-client",
		"apt-cache show gosu",
	} {
		if !strings.Contains(script, snippet) {
			t.Fatalf("setup-debian-apt-mirrors.sh 必须保留 %q", snippet)
		}
	}
	if strings.Contains(script, "https://mirrors.aliyun.com") {
		t.Fatal("setup-debian-apt-mirrors.sh 不能在安装 ca-certificates 之前写 https 镜像")
	}
}

func TestPythonRuntimeFetchAvoidsGitHubHTTP2Drops(t *testing.T) {
	text := readPythonRuntimeInstaller(t)
	for _, snippet := range []string{
		"--http1.1",
		"-C -",
		"https://gh-proxy.org/",
		"https://ghfast.top/",
		"attempt ${attempt}",
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("install-python-runtimes.sh 必须保留 %q，避免国内机器直连 GitHub Release 被掐断后整段重下", snippet)
		}
	}
	if strings.Contains(text, "--retry-all-errors") {
		t.Fatal("install-python-runtimes.sh 不能再用 curl --retry-all-errors，它会和 -C - 打架并把已下载的半截扔掉")
	}
	if strings.Contains(text, `curl -fsSL "$url" -o "$out"`) {
		t.Fatal("install-python-runtimes.sh 不能再回到单次直连 curl，那条路会把 30 分钟下载浪费在 PROTOCOL_ERROR 上")
	}
}
