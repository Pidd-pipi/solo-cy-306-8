<template>
  <div class="group-signup">
    <el-alert type="info" :closable="false" class="mb-2">
      <template #title>
        团体报名：一次填写 {{ minSize }}~{{ maxSize }} 名参加人，名额按整团计算，剩余名额不足时整团无法报名。
      </template>
    </el-alert>
    <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
      <el-table :data="form.members" border size="small">
        <el-table-column label="#" type="index" width="48" />
        <el-table-column label="姓名" min-width="120">
          <template #default="{ $index }">
            <el-form-item :prop="`members.${$index}.name`" :rules="nameRules" class="cell-item">
              <el-input v-model="form.members[$index].name" placeholder="参加人姓名" maxlength="50" />
            </el-form-item>
          </template>
        </el-table-column>
        <el-table-column label="手机号" min-width="150">
          <template #default="{ $index }">
            <el-form-item :prop="`members.${$index}.phone`" :rules="phoneRules" class="cell-item">
              <el-input v-model="form.members[$index].phone" placeholder="手机号" maxlength="20" />
            </el-form-item>
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="140">
          <template #default="{ $index }">
            <el-input v-model="form.members[$index].remark" placeholder="选填" maxlength="255" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" v-if="form.members.length > minSize">
          <template #default="{ $index }">
            <el-button link type="danger" size="small" @click="removeMember($index)">移除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-button class="mt-2" size="small" :disabled="form.members.length >= maxSize" @click="addMember">
        + 添加参加人
      </el-button>
      <div class="footer mt-2">
        <span class="count">当前 {{ form.members.length }} 人，将占用 {{ form.members.length }} 个名额</span>
        <el-button type="primary" :loading="loading" @click="submit">整团报名</el-button>
      </div>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { groupSignup } from '@/api/groupRegistration'
import type { GroupMemberInput } from '@/types'

const props = defineProps<{
  activityId: number
  groupMaxSize: number
  remaining: number
}>()
const emit = defineEmits<{ (e: 'success', vouchers: string[]): void }>()

const formRef = ref<FormInstance>()
const loading = ref(false)
const minSize = 2
const maxSize = computed(() => Math.max(props.groupMaxSize, minSize))

const emptyMember = (): GroupMemberInput => ({ name: '', phone: '', remark: '' })
const form = reactive<{ members: GroupMemberInput[] }>({
  members: [emptyMember(), emptyMember()],
})

const nameRules = [{ required: true, message: '请输入姓名', trigger: 'blur' }]
const phoneRules = [{ required: true, message: '请输入手机号', trigger: 'blur' }]
const rules: FormRules = {}

function addMember() {
  if (form.members.length >= maxSize.value) return
  form.members.push(emptyMember())
}
function removeMember(index: number) {
  form.members.splice(index, 1)
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  if (form.members.length < minSize) {
    ElMessage.warning(`团体报名至少需要 ${minSize} 名参加人`)
    return
  }
  if (props.remaining > 0 && form.members.length > props.remaining) {
    ElMessage.warning(`剩余名额仅 ${props.remaining} 个，不足 ${form.members.length} 人整团，请减少人数后再试`)
    return
  }
  const phones = form.members.map((m) => m.phone.trim())
  if (new Set(phones).size !== phones.length) {
    ElMessage.warning('团内手机号不能重复')
    return
  }
  loading.value = true
  try {
    const res = await groupSignup({
      activity_id: props.activityId,
      members: form.members.map((m) => ({ ...m, name: m.name.trim(), phone: m.phone.trim(), remark: m.remark?.trim() ?? '' })),
    })
    ElMessage.success(`团体报名成功，共 ${res.data.members.length} 人`)
    emit(
      'success',
      res.data.members.map((m: { voucher_no: string }) => m.voucher_no),
    )
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.mb-2 { margin-bottom: 12px; }
.mt-2 { margin-top: 12px; }
.cell-item { margin-bottom: 0; }
.footer { display: flex; justify-content: space-between; align-items: center; }
.count { color: #606266; font-size: 13px; }
</style>
