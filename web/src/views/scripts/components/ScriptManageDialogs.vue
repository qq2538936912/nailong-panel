<script setup lang="ts">
import { FolderAdd, Upload } from '@element-plus/icons-vue'
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import type { ScriptUploadMode, ScriptVersionRecord } from '../types'

const MonacoDiffEditor = defineAsyncComponent(() => import('@/components/MonacoDiffEditor.vue'))

const showCreateFileDialog = defineModel<boolean>('showCreateFileDialog', { required: true })
const showCreateDirDialog = defineModel<boolean>('showCreateDirDialog', { required: true })
const showRenameDialog = defineModel<boolean>('showRenameDialog', { required: true })
const showVersionDialog = defineModel<boolean>('showVersionDialog', { required: true })
const showVersionDiffDialog = defineModel<boolean>('showVersionDiffDialog', { required: true })
const showUploadDialog = defineModel<boolean>('showUploadDialog', { required: true })

const newFileName = defineModel<string>('newFileName', { required: true })
const newFileParent = defineModel<string>('newFileParent', { required: true })
const newDirName = defineModel<string>('newDirName', { required: true })
const newDirParent = defineModel<string>('newDirParent', { required: true })
const renameTarget = defineModel<string>('renameTarget', { required: true })
const uploadDir = defineModel<string>('uploadDir', { required: true })
const uploadMode = defineModel<ScriptUploadMode>('uploadMode', { required: true })
const versionDiffOriginalTitle = defineModel<string>('versionDiffOriginalTitle', { required: true })
const versionDiffModifiedTitle = defineModel<string>('versionDiffModifiedTitle', { required: true })
const versionDiffOriginalContent = defineModel<string>('versionDiffOriginalContent', { required: true })
const versionDiffModifiedContent = defineModel<string>('versionDiffModifiedContent', { required: true })

const props = defineProps<{
  isMobile: boolean
  selectedFile: string
  allFolders: string[]
  editorLanguage: string
  versions: ScriptVersionRecord[]
  versionsLoading: boolean
  versionDiffLoading: boolean
  onCreateFile: () => void | Promise<void>
  onCreateDir: () => void | Promise<void>
  onRename: () => void | Promise<void>
  onCompareVersion: (version: ScriptVersionRecord) => void | Promise<void>
  onRollback: (versionId: number) => void | Promise<void>
  onClearVersions: () => void | Promise<void>
  onUploadFileChange: (file: any, files: any[]) => void
  onUploadSubmit: () => void | Promise<void>
}>()

// 移动/复制的目标目录下拉同样要排掉受控目录，
// 否则用户能在这里选中 .git 当目标（后端会直接拒绝，白跑一趟）。
const hiddenTargetFolderSegments = new Set(['node_modules', '__pycache__', '.git', '.svn', '.hg', '.bzr'])
const nestedFolders = computed(() => props.allFolders.filter(folder => folder && !folder.split('/').some(segment => hiddenTargetFolderSegments.has(segment.trim().toLowerCase()))))
const ignoreWhitespaceDiff = ref(false)
const folderInputRef = ref<HTMLInputElement>()
const selectedUploadCount = ref(0)
const selectedFolderName = ref('')

const uploadDialogTitle = computed(() => uploadMode.value === 'folder' ? '上传文件夹' : '上传文件')

function resetUploadSelection() {
  selectedUploadCount.value = 0
  selectedFolderName.value = ''
  if (folderInputRef.value) {
    folderInputRef.value.value = ''
  }
}

function handleUploadFileChange(file: any, files: any[]) {
  selectedUploadCount.value = files.length
  selectedFolderName.value = ''
  props.onUploadFileChange(file, files)
}

function handleFolderInputChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  selectedUploadCount.value = files.length
  selectedFolderName.value = files[0]?.webkitRelativePath?.split('/')[0] || ''
  props.onUploadFileChange(undefined, files.map((raw) => ({ raw })))
}

function openFolderPicker() {
  folderInputRef.value?.click()
}

watch(showUploadDialog, (visible) => {
  if (!visible) {
    resetUploadSelection()
  }
})

watch(uploadMode, () => {
  resetUploadSelection()
  props.onUploadFileChange(undefined, [])
})

function normalizeWhitespaceForCompare(content: string) {
  return content.replace(/\s+/g, '')
}

const versionDiffExactSame = computed(() => versionDiffOriginalContent.value === versionDiffModifiedContent.value)
const versionDiffWhitespaceOnly = computed(() =>
  !versionDiffExactSame.value &&
  normalizeWhitespaceForCompare(versionDiffOriginalContent.value) === normalizeWhitespaceForCompare(versionDiffModifiedContent.value)
)
const versionDiffEffectivelySame = computed(() =>
  versionDiffExactSame.value || (ignoreWhitespaceDiff.value && versionDiffWhitespaceOnly.value)
)
const versionDiffSummaryTitle = computed(() => {
  if (versionDiffExactSame.value) {
    return '当前版本与历史版本一致'
  }
  if (ignoreWhitespaceDiff.value && versionDiffWhitespaceOnly.value) {
    return '当前版本与历史版本仅存在空白差异'
  }
  return ''
})
const versionDiffSummaryDescription = computed(() => {
  if (versionDiffExactSame.value) {
    return '这两个版本的代码内容完全相同，无需继续查看差异。'
  }
  if (ignoreWhitespaceDiff.value && versionDiffWhitespaceOnly.value) {
    return '已开启忽略空白差异，当前只检测到空格、缩进或换行变化。关闭开关后可查看完整差异。'
  }
  return ''
})

watch(showVersionDiffDialog, (visible) => {
  if (!visible) {
    ignoreWhitespaceDiff.value = false
  }
})
</script>

<template>
  <el-dialog v-model="showCreateFileDialog" title="新建文件" :width="isMobile ? '90%' : '480px'" :fullscreen="isMobile">
    <el-form :label-width="isMobile ? 'auto' : '80px'" :label-position="isMobile ? 'top' : 'right'">
      <el-form-item label="上级目录">
        <!-- 目录很多时允许直接输入关键词筛选，避免用户只能滚动查找。 -->
        <el-select
          v-model="newFileParent"
          placeholder="输入关键词搜索目录，留空为根目录"
          clearable
          filterable
          default-first-option
          no-match-text="没有匹配的目录"
          no-data-text="暂无可选目录"
          style="width: 100%"
        >
          <el-option label="根目录" value="" />
          <el-option v-for="folder in nestedFolders" :key="folder" :label="folder" :value="folder" />
        </el-select>
      </el-form-item>
      <el-form-item label="文件名">
        <el-input v-model="newFileName" placeholder="如: script.py / main.go / task.sh" @keyup.enter="onCreateFile" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showCreateFileDialog = false">取消</el-button>
      <el-button type="primary" @click="onCreateFile">创建</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="showCreateDirDialog" title="新建目录" :width="isMobile ? '90%' : '480px'" :fullscreen="isMobile">
    <el-form :label-width="isMobile ? 'auto' : '80px'" :label-position="isMobile ? 'top' : 'right'">
      <el-form-item label="上级目录">
        <!-- 目录很多时允许直接输入关键词筛选，避免用户只能滚动查找。 -->
        <el-select
          v-model="newDirParent"
          placeholder="输入关键词搜索目录，留空为根目录"
          clearable
          filterable
          default-first-option
          no-match-text="没有匹配的目录"
          no-data-text="暂无可选目录"
          style="width: 100%"
        >
          <el-option label="根目录" value="" />
          <el-option v-for="folder in nestedFolders" :key="folder" :label="folder" :value="folder" />
        </el-select>
      </el-form-item>
      <el-form-item label="目录名">
        <el-input v-model="newDirName" placeholder="如: utils" @keyup.enter="onCreateDir" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showCreateDirDialog = false">取消</el-button>
      <el-button type="primary" @click="onCreateDir">创建</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="showRenameDialog" title="重命名" :width="isMobile ? '90%' : '400px'" :fullscreen="isMobile">
    <el-input v-model="renameTarget" placeholder="新名称" @keyup.enter="onRename" />
    <template #footer>
      <el-button @click="showRenameDialog = false">取消</el-button>
      <el-button type="primary" @click="onRename">确定</el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="showVersionDialog"
    title="版本历史"
    :width="isMobile ? '100%' : 'min(1180px, 94vw)'"
    :fullscreen="isMobile"
    :top="isMobile ? '0' : '5vh'"
  >
    <div class="version-history-toolbar">
      <div class="version-history-meta">
        <strong>{{ props.selectedFile || '当前脚本' }}</strong>
        <span>
          {{ props.versions.length > 0 ? `共 ${props.versions.length} 条版本记录` : '当前脚本还没有版本记录' }}
        </span>
      </div>
      <el-button
        type="danger"
        plain
        :disabled="props.versionsLoading || !props.selectedFile || props.versions.length === 0"
        @click="props.onClearVersions"
      >
        清空版本历史
      </el-button>
    </div>
    <el-table
      :data="versions"
      v-loading="versionsLoading"
      :max-height="isMobile ? 'calc(100dvh - 240px)' : 'calc(100dvh - 320px)'"
      table-layout="auto"
      class="version-history-table"
    >
      <el-table-column prop="version" label="版本" width="88" />
      <el-table-column prop="message" label="备注" min-width="260" show-overflow-tooltip />
      <el-table-column prop="content_length" label="大小" width="120">
        <template #default="{ row }">{{ (row.content_length / 1024).toFixed(1) }} KB</template>
      </el-table-column>
      <el-table-column prop="created_at" label="时间" width="188">
        <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
      </el-table-column>
      <el-table-column label="操作" width="156" fixed="right">
        <template #default="{ row }">
          <el-button size="small" text type="primary" @click="onCompareVersion(row)">对比</el-button>
          <el-button size="small" text type="primary" @click="onRollback(row.id)">回滚</el-button>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty description="当前脚本还没有版本记录" />
      </template>
    </el-table>
  </el-dialog>

  <el-dialog
    v-model="showVersionDiffDialog"
    title="版本差异对比"
    :width="isMobile ? '100%' : '92%'"
    :fullscreen="isMobile"
    :top="isMobile ? '0' : '4vh'"
    :close-on-click-modal="false"
    append-to-body
    destroy-on-close
  >
    <div
      class="version-diff-dialog"
      v-loading="props.versionDiffLoading"
      element-loading-text="正在加载历史版本内容..."
    >
      <div class="version-diff-header">
        <div class="version-diff-side">
          <span class="version-diff-caption">左侧：历史代码</span>
          <strong>{{ versionDiffOriginalTitle || '历史版本' }}</strong>
        </div>
        <div class="version-diff-side version-diff-side--current">
          <span class="version-diff-caption">右侧：当前代码</span>
          <strong>{{ versionDiffModifiedTitle || '当前代码' }}</strong>
        </div>
      </div>
      <div class="version-diff-toolbar">
        <div v-if="versionDiffSummaryTitle" class="version-diff-summary">
          <strong>{{ versionDiffSummaryTitle }}</strong>
          <span>{{ versionDiffSummaryDescription }}</span>
        </div>
        <div class="version-diff-switch">
          <span>忽略空白差异</span>
          <el-switch
            v-model="ignoreWhitespaceDiff"
            inline-prompt
            active-text="开"
            inactive-text="关"
          />
        </div>
      </div>
      <div v-if="!props.versionDiffLoading && versionDiffEffectivelySame" class="version-diff-empty">
        <el-empty :description="versionDiffSummaryTitle || '当前版本与历史版本一致'">
          <template #description>
            <div class="version-diff-empty-copy">
              <strong>{{ versionDiffSummaryTitle || '当前版本与历史版本一致' }}</strong>
              <span>{{ versionDiffSummaryDescription || '这两个版本没有可显示的差异。' }}</span>
            </div>
          </template>
        </el-empty>
      </div>
      <MonacoDiffEditor
        v-else-if="!props.versionDiffLoading"
        :original-value="versionDiffOriginalContent"
        :modified-value="versionDiffModifiedContent"
        :language="props.editorLanguage"
        :render-side-by-side="!isMobile"
        :ignore-trim-whitespace="ignoreWhitespaceDiff"
        class="version-diff-editor"
      />
    </div>
  </el-dialog>

  <el-dialog v-model="showUploadDialog" :title="uploadDialogTitle" :width="isMobile ? '90%' : '520px'" :fullscreen="isMobile" destroy-on-close>
    <el-form :label-width="isMobile ? 'auto' : '80px'" :label-position="isMobile ? 'top' : 'right'">
      <el-form-item label="上传方式">
        <el-radio-group v-model="uploadMode">
          <el-radio-button value="file">上传文件</el-radio-button>
          <el-radio-button value="folder">上传文件夹</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="目标目录">
        <!-- 上传目标目录也支持输入关键词筛选，和新建文件/目录保持一致。 -->
        <el-select
          v-model="uploadDir"
          placeholder="输入关键词搜索目录，留空为根目录"
          clearable
          filterable
          default-first-option
          no-match-text="没有匹配的目录"
          no-data-text="暂无可选目录"
          style="width: 100%"
        >
          <el-option label="根目录" value="" />
          <el-option v-for="folder in nestedFolders" :key="folder" :label="folder" :value="folder" />
        </el-select>
      </el-form-item>
      <el-form-item :label="uploadMode === 'folder' ? '选择文件夹' : '选择文件'">
        <el-upload
          v-if="uploadMode === 'file'"
          :auto-upload="false"
          :show-file-list="true"
          multiple
          :on-change="handleUploadFileChange"
          :on-remove="handleUploadFileChange"
          drag
        >
          <el-icon :size="40"><Upload /></el-icon>
          <div>拖拽文件到此处或点击选择</div>
          <div class="el-upload__tip">支持一次选择多个脚本文件，单个文件大小不超过 100MB。</div>
        </el-upload>
        <div v-else class="folder-upload">
          <input
            ref="folderInputRef"
            type="file"
            class="folder-upload-input"
            webkitdirectory
            multiple
            @change="handleFolderInputChange"
          />
          <button type="button" class="folder-upload-drop" @click="openFolderPicker">
            <el-icon :size="40"><FolderAdd /></el-icon>
            <div>点击选择文件夹</div>
            <div class="el-upload__tip">会保留文件夹内的相对路径；单个文件不超过 100MB。</div>
          </button>
          <div v-if="selectedUploadCount > 0" class="folder-upload-summary">
            已选择 {{ selectedFolderName ? `${selectedFolderName} / ` : '' }}{{ selectedUploadCount }} 个文件
          </div>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showUploadDialog = false">取消</el-button>
      <el-button type="primary" @click="onUploadSubmit">上传</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.folder-upload {
  width: 100%;
}

.folder-upload-input {
  display: none;
}

.folder-upload-drop {
  width: 100%;
  min-height: 148px;
  padding: 24px 16px;
  border: 1px dashed var(--el-border-color);
  background: var(--el-fill-color-lighter);
  color: var(--el-text-color-regular);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  cursor: pointer;
  font: inherit;
}

.folder-upload-drop:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}

.folder-upload-summary {
  margin-top: 10px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  line-height: 1.6;
}

.version-history-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
  flex-wrap: wrap;
}

.version-history-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;

  strong {
    font-size: 14px;
    font-weight: 700;
    color: var(--el-text-color-primary);
    line-height: 1.5;
    word-break: break-all;
  }

  span {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 1.6;
  }
}

.version-history-table {
  :deep(.el-empty) {
    padding: 32px 0;
  }
}

.version-diff-dialog {
  display: flex;
  flex-direction: column;
  height: calc(92vh - 80px);
}

.version-diff-header {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.version-diff-side {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 0;
  background: var(--el-fill-color-light);
}

.version-diff-side--current {
  text-align: right;
}

.version-diff-caption {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.version-diff-editor {
  flex: 1 1 0;
  min-height: 0;
  overflow: hidden;
}

.version-diff-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.version-diff-summary {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;

  strong {
    font-size: 13px;
    font-weight: 700;
    color: var(--el-text-color-primary);
  }

  span {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    line-height: 1.6;
  }
}

.version-diff-switch {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 0;
  background: var(--el-fill-color-light);
  color: var(--el-text-color-secondary);
  font-size: 12px;
  font-weight: 600;
}

.version-diff-empty {
  flex: 1;
  min-height: 62vh;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px dashed var(--el-border-color);
  border-radius: 0;
  background: color-mix(in srgb, var(--el-fill-color-light) 72%, white);
}

.version-diff-empty-copy {
  display: flex;
  flex-direction: column;
  gap: 6px;

  strong {
    font-size: 15px;
    font-weight: 700;
    color: var(--el-text-color-primary);
  }

  span {
    font-size: 13px;
    line-height: 1.7;
    color: var(--el-text-color-secondary);
  }
}

@media (max-width: 768px) {
  .version-history-toolbar {
    align-items: stretch;
  }

  .version-diff-header {
    grid-template-columns: 1fr;
  }

  .version-diff-side--current {
    text-align: left;
  }

  .version-diff-editor {
    min-height: calc(100dvh - 220px);
  }

  .version-diff-toolbar {
    align-items: stretch;
  }

  .version-diff-switch {
    justify-content: space-between;
  }

  .version-diff-empty {
    min-height: calc(100dvh - 220px);
  }
}
</style>
