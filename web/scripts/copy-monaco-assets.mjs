import fs from 'node:fs'
import path from 'node:path'

const sourceDir = path.resolve(process.cwd(), 'node_modules/monaco-editor/min')
const targetDir = path.resolve(process.cwd(), 'dist/monaco')
const sourceVsDir = path.join(sourceDir, 'vs')
const targetVsDir = path.join(targetDir, 'vs')

/** 整目录保留：editor / language / basic-languages 体量小且路径稳定。 */
const COPY_DIRECTORIES = ['editor', 'language', 'basic-languages']

/**
 * assets 里按前缀保留 worker。刻意不拷贝 css/html worker（各 ~1MB）：
 * 脚本页偶尔编辑 .css/.html 仍保留语法高亮，只是没有语言服务/校验。
 */
const ASSET_WORKER_PREFIXES = ['editor.worker-', 'ts.worker-', 'json.worker-']

/** vs 根目录下按文件名模式保留的语言/runtime 分块（hash 会变，所以用正则而不是白名单文件名）。 */
const KEEP_VS_ROOT_PATTERNS = [
  /^loader\.js$/,
  /^editor\.api-.*\.js$/,
  /^workers-.*\.js$/,
  /^_commonjsHelpers-.*\.js$/,
  /^nls\.messages-loader\.js$/,
  /^nls\.messages\.js\.js$/,
  /^nls\.messages\.zh-cn\.js\.js$/,
  /^tsMode-.*\.js$/,
  /^jsonMode-.*\.js$/,
  /^cssMode-.*\.js$/,
  /^htmlMode-.*\.js$/,
  /^javascript-.*\.js$/,
  /^typescript-.*\.js$/,
  /^python-.*\.js$/,
  /^shell-.*\.js$/,
  /^yaml-.*\.js$/,
  /^markdown-.*\.js$/,
  /^go-.*\.js$/,
  /^css-.*\.js$/,
  /^html-.*\.js$/,
  /^xml-.*\.js$/,
  /^monaco\.contribution-.*\.js$/,
  /^lspLanguageFeatures-.*\.js$/,
]

/** 构建后必须存在的路径模式（相对 dist/monaco/），缺任意一条直接 fail build。 */
const REQUIRED_RELATIVE_PATTERNS = [
  /^vs\/loader\.js$/,
  /^vs\/editor\/editor\.main\.js$/,
  /^vs\/editor\/editor\.main\.css$/,
  /^vs\/editor\.api-.*\.js$/,
  /^vs\/language\/css\/monaco\.contribution\.js$/,
  /^vs\/language\/html\/monaco\.contribution\.js$/,
  /^vs\/language\/json\/monaco\.contribution\.js$/,
  /^vs\/language\/typescript\/monaco\.contribution\.js$/,
  /^vs\/assets\/editor\.worker-.*\.js$/,
  /^vs\/assets\/ts\.worker-.*\.js$/,
  /^vs\/assets\/json\.worker-.*\.js$/,
  /^vs\/python-.*\.js$/,
  /^vs\/shell-.*\.js$/,
]

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true })
}

function copyFile(sourcePath, targetPath) {
  ensureDir(path.dirname(targetPath))
  fs.copyFileSync(sourcePath, targetPath)
}

function shouldKeepAssetWorker(fileName) {
  return ASSET_WORKER_PREFIXES.some((prefix) => fileName.startsWith(prefix))
}

function shouldKeepVsRootFile(fileName) {
  return KEEP_VS_ROOT_PATTERNS.some((pattern) => pattern.test(fileName))
}

function copyDirectoryRecursive(source, target) {
  ensureDir(target)
  for (const entry of fs.readdirSync(source, { withFileTypes: true })) {
    const sourcePath = path.join(source, entry.name)
    const targetPath = path.join(target, entry.name)
    if (entry.isDirectory()) {
      copyDirectoryRecursive(sourcePath, targetPath)
      continue
    }
    copyFile(sourcePath, targetPath)
  }
}

function listRelativeFiles(rootDir, currentDir = rootDir) {
  const entries = fs.readdirSync(currentDir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const absolutePath = path.join(currentDir, entry.name)
    if (entry.isDirectory()) {
      files.push(...listRelativeFiles(rootDir, absolutePath))
      continue
    }
    files.push(path.relative(rootDir, absolutePath))
  }
  return files
}

function sumDirectoryBytes(rootDir) {
  let total = 0
  for (const relativePath of listRelativeFiles(rootDir)) {
    total += fs.statSync(path.join(rootDir, relativePath)).size
  }
  return total
}

function copyMonacoAssets() {
  if (!fs.existsSync(sourceVsDir)) {
    throw new Error('[copy-monaco-assets] 找不到 node_modules/monaco-editor/min/vs')
  }

  ensureDir(targetVsDir)

  for (const dirName of COPY_DIRECTORIES) {
    copyDirectoryRecursive(path.join(sourceVsDir, dirName), path.join(targetVsDir, dirName))
  }

  for (const entry of fs.readdirSync(path.join(sourceVsDir, 'assets'), { withFileTypes: true })) {
    if (!entry.isFile() || !shouldKeepAssetWorker(entry.name)) {
      continue
    }
    copyFile(path.join(sourceVsDir, 'assets', entry.name), path.join(targetVsDir, 'assets', entry.name))
  }

  for (const entry of fs.readdirSync(sourceVsDir, { withFileTypes: true })) {
    if (!entry.isFile() || !shouldKeepVsRootFile(entry.name)) {
      continue
    }
    copyFile(path.join(sourceVsDir, entry.name), path.join(targetVsDir, entry.name))
  }

  const copiedFiles = listRelativeFiles(targetDir)
  const missingPatterns = REQUIRED_RELATIVE_PATTERNS.filter(
    (pattern) => !copiedFiles.some((relativePath) => pattern.test(relativePath.replace(/\\/g, '/'))),
  )

  if (missingPatterns.length > 0) {
    throw new Error(
      `[copy-monaco-assets] 裁剪后缺少运行时必需文件：\n  ${missingPatterns.map((pattern) => pattern.toString()).join('\n  ')}`,
    )
  }

  const probeManifest = copiedFiles
    .map((relativePath) => relativePath.replace(/\\/g, '/'))
    .filter((relativePath) =>
      [
        'vs/loader.js',
        'vs/editor/editor.main.js',
        'vs/editor/editor.main.css',
        'vs/language/css/monaco.contribution.js',
        'vs/language/html/monaco.contribution.js',
        'vs/language/json/monaco.contribution.js',
        'vs/language/typescript/monaco.contribution.js',
      ].includes(relativePath) || /^vs\/editor\.api-.*\.js$/.test(relativePath),
    )

  fs.writeFileSync(
    path.join(targetDir, 'probe-manifest.json'),
    JSON.stringify(probeManifest, null, 2),
  )

  const copiedBytes = sumDirectoryBytes(targetDir)
  const skippedBytes = sumDirectoryBytes(sourceDir) - copiedBytes

  console.log(
    `[copy-monaco-assets] copied ${(copiedBytes / 1024 / 1024).toFixed(2)} MB, ` +
      `skipped ${(skippedBytes / 1024 / 1024).toFixed(2)} MB ` +
      `(monaco-editor/min -> dist/monaco)`,
  )
}

copyMonacoAssets()
