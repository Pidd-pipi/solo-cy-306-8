<template>
  <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
    <el-form-item label="活动标题" prop="title">
      <el-input v-model="form.title" />
    </el-form-item>
    <el-form-item label="活动类型" prop="activity_type">
      <el-select v-model="form.activity_type" style="width: 100%">
        <el-option v-for="opt in ActivityTypeOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
      </el-select>
    </el-form-item>
    <el-form-item label="活动描述">
      <el-input v-model="form.description" type="textarea" :rows="3" />
    </el-form-item>
    <el-form-item label="封面图">
      <ImageUploader v-model="form.cover_image" />
    </el-form-item>
    <el-form-item label="开始时间" prop="start_time">
      <el-date-picker v-model="form.start_time" type="datetime" style="width: 100%" />
    </el-form-item>
    <el-form-item label="结束时间" prop="end_time">
      <el-date-picker v-model="form.end_time" type="datetime" style="width: 100%" />
    </el-form-item>
    <el-form-item label="报名截止" prop="signup_deadline">
      <el-date-picker v-model="form.signup_deadline" type="datetime" style="width: 100%" />
    </el-form-item>
    <el-form-item label="地点">
      <el-input v-model="form.location" />
    </el-form-item>
    <el-form-item label="名额">
      <el-input-number v-model="form.capacity" :min="0" />
      <span class="hint">{{ form.capacity > 0 ? `共 ${form.capacity} 个名额（0 表示不限）` : '不限名额' }}</span>
    </el-form-item>
    <el-form-item label="团体报名">
      <el-switch v-model="form.group_signup_enabled" />
      <span class="hint">开启后该活动仅支持整团报名，每团至少 2 人</span>
    </el-form-item>
    <el-form-item v-if="form.group_signup_enabled" label="单团人数上限" prop="group_max_size">
      <el-input-number v-model="form.group_max_size" :min="2" :max="groupMaxUpperBound" />
      <span class="hint">每个团体最多 {{ form.group_max_size }} 人</span>
    </el-form-item>
    <el-form-item>
      <el-button type="primary" :loading="loading" @click="submit">{{ form.id ? '保存' : '创建' }}</el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import ImageUploader from '@/components/common/ImageUploader.vue'
import { createActivity, updateActivity } from '@/api/activity'
import { ActivityTypeOptions } from '@/constants/activity'

const props = defineProps<{ activity?: any }>()
const emit = defineEmits<{ (e: 'success'): void }>()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({
  id: props.activity?.id || 0,
  title: props.activity?.title || '',
  description: props.activity?.description || '',
  cover_image: props.activity?.cover_image || '',
  activity_type: props.activity?.activity_type || 'lecture',
  start_time: props.activity?.start_time || '',
  end_time: props.activity?.end_time || '',
  signup_deadline: props.activity?.signup_deadline || '',
  location: props.activity?.location || '',
  capacity: props.activity?.capacity ?? 100,
  group_signup_enabled: props.activity?.group_signup_enabled ?? false,
  group_max_size: props.activity?.group_max_size ?? 2,
})

// 有名额限制时单团上限不能超过活动总名额；不限名额（0）时上界取 100。
const groupMaxUpperBound = computed(() => (form.capacity > 0 ? form.capacity : 100))

watch(
  () => form.group_signup_enabled,
  (enabled) => {
    if (enabled && form.group_max_size < 2) form.group_max_size = 2
    if (form.group_max_size > groupMaxUpperBound.value) form.group_max_size = groupMaxUpperBound.value
  },
)
watch(groupMaxUpperBound, (bound) => {
  if (form.group_signup_enabled && form.group_max_size > bound) form.group_max_size = bound
})

const rules: FormRules = {
  title: [{ required: true, message: '请输入活动标题', trigger: 'blur' }],
  activity_type: [{ required: true, message: '请选择活动类型', trigger: 'change' }],
  start_time: [{ required: true, message: '请选择开始时间', trigger: 'change' }],
  end_time: [{ required: true, message: '请选择结束时间', trigger: 'change' }],
  signup_deadline: [{ required: true, message: '请选择报名截止时间', trigger: 'change' }],
  group_max_size: [
    {
      validator: (_rule: unknown, value: number, callback: (e?: Error) => void) => {
        if (form.group_signup_enabled && (!value || value < 2)) {
          callback(new Error('单团人数上限至少为 2'))
        } else if (form.group_signup_enabled && form.capacity > 0 && value > form.capacity) {
          callback(new Error('单团人数上限不能超过活动名额'))
        } else {
          callback()
        }
      },
      trigger: 'change',
    },
  ],
}

async function submit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    const payload = { ...form }
    if (!payload.group_signup_enabled) payload.group_max_size = 0
    if (form.id) {
      await updateActivity(form.id, payload)
    } else {
      await createActivity(payload)
    }
    ElMessage.success('保存成功')
    emit('success')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.hint { margin-left: 10px; color: #909399; font-size: 12px; }
</style>
