import request from '@/utils/request'
import type { GroupMemberInput, GroupView } from '@/types'

// 团体报名：一次提交多名参加人（至少 2 人），每人各自生成入场凭证号。
export function groupSignup(data: { activity_id: number; members: GroupMemberInput[] }) {
  return request.post('/registration-groups', data)
}

export function listMyGroups(params: { page?: number; page_size?: number }) {
  return request.get('/registration-groups/mine', { params })
}

export function getGroup(id: number) {
  return request.get(`/registration-groups/${id}`)
}

export function cancelGroup(id: number) {
  return request.post(`/registration-groups/${id}/cancel`)
}

export type { GroupView }
