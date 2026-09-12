<template>
  <div v-loading="loading">
    <el-empty v-if="!loading && groups.length === 0" description="暂无团体报名" :image-size="80" />
    <el-collapse v-else v-model="active" accordion>
      <el-collapse-item v-for="gv in groups" :key="gv.group.id" :name="String(gv.group.id)">
        <template #title>
          <span class="title">
            团号 {{ gv.group.id }}
            <el-tag size="small" class="ml-1" :type="gv.group.status === 'registered' ? 'success' : 'info'">
              {{ GroupStatusText[gv.group.status] || gv.group.status }}
            </el-tag>
            <span class="sub">活动 #{{ gv.group.activity_id }} · {{ gv.group.member_count }} 人 · {{ gv.group.created_at }}</span>
          </span>
        </template>
        <el-table :data="gv.members" border size="small">
          <el-table-column type="index" label="#" width="48" />
          <el-table-column prop="name" label="姓名" width="110" />
          <el-table-column prop="phone" label="手机号" width="140" />
          <el-table-column prop="voucher_no" label="入场凭证号" min-width="180" />
          <el-table-column prop="remark" label="备注" min-width="100" />
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="memberTag(row.status)">{{ RegistrationStatusText[row.status] || row.status }}</el-tag>
            </template>
          </el-table-column>
        </el-table>
        <div class="actions">
          <el-button
            v-if="gv.group.status === 'registered'"
            size="small"
            type="danger"
            :loading="cancellingId === gv.group.id"
            @click="cancel(gv.group.id)"
          >
            整团取消
          </el-button>
        </div>
      </el-collapse-item>
    </el-collapse>
    <el-pagination
      class="mt-2"
      layout="total, prev, pager, next"
      :total="total"
      :page-size="pageSize"
      @current-change="onPage"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { listMyGroups, cancelGroup } from '@/api/groupRegistration'
import { RegistrationStatusText } from '@/constants/registration'
import type { GroupView } from '@/types'

const GroupStatusText: Record<string, string> = {
  registered: '已报名',
  cancelled: '已取消',
}

const groups = ref<GroupView[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const pageSize = 10
const active = ref('')
const cancellingId = ref(0)

async function load() {
  loading.value = true
  try {
    const res = await listMyGroups({ page: page.value, page_size: pageSize })
    groups.value = res.data.list
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

async function cancel(id: number) {
  await ElMessageBox.confirm('将整团取消（团内全部成员一并取消），确认继续？', '整团取消', { type: 'warning' })
  cancellingId.value = id
  try {
    await cancelGroup(id)
    ElMessage.success('团体报名已取消')
    await load()
  } finally {
    cancellingId.value = 0
  }
}

function onPage(p: number) {
  page.value = p
  load()
}

function memberTag(status: string): string {
  if (status === 'checked_in') return 'success'
  if (status === 'cancelled') return 'info'
  return 'primary'
}

onMounted(load)
</script>

<style scoped>
.title { display: inline-flex; align-items: center; }
.ml-1 { margin-left: 6px; }
.sub { margin-left: 10px; color: #909399; font-size: 12px; font-weight: 400; }
.actions { margin-top: 10px; text-align: right; }
.mt-2 { margin-top: 12px; }
</style>
