// 未知问题运营路由（T1 低置信度回流）
export default [
  {
    path: '/unknown-questions',
    name: 'UnknownQuestionList',
    component: () => import('@/views/unknownQuestion/List.vue'),
    meta: { title: '未知问题运营', group: 'customerservice', icon: 'QuestionFilled', requiresAuth: true }
  }
]
