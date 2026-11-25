// Client subscription DNS labels. Keep protocol names and native JSON keys unchanged.
const en = {
  legacy: 'Legacy', up: 'Move up', down: 'Move down',
  automatic: 'Automatic', invalid: 'Invalid DNS configuration',
  ruleHelp: 'Set the action, target and match conditions in JSON. Without match conditions the rule may match all queries.',
  invalidJson: 'Enter a valid JSON object.',
}
type Labels = Record<keyof typeof en, string>
export const subscriptionDnsMessages = {
  en,
  ru: {
    legacy: 'Старый формат', up: 'Выше', down: 'Ниже',
    automatic: 'Автоматически', invalid: 'Ошибка конфигурации DNS',
    ruleHelp: 'Укажите действие, цель и условия в JSON. Правило без условий может совпадать со всеми запросами.',
    invalidJson: 'Введите корректный объект JSON.',
  },
  fa: {
    legacy: 'قالب قدیمی', up: 'انتقال به بالا', down: 'انتقال به پایین',
    automatic: 'خودکار', invalid: 'پیکربندی DNS نامعتبر است',
    ruleHelp: 'عمل، مقصد و شرایط تطبیق را در JSON تعیین کنید. قانون بدون شرط ممکن است با همهٔ درخواست‌ها تطبیق داشته باشد.',
    invalidJson: 'یک شیء JSON معتبر وارد کنید.',
  },
  vi: {
    legacy: 'Định dạng cũ', up: 'Di chuyển lên', down: 'Di chuyển xuống',
    automatic: 'Tự động', invalid: 'Cấu hình DNS không hợp lệ',
    ruleHelp: 'Đặt hành động, đích và điều kiện khớp trong JSON. Quy tắc không có điều kiện có thể khớp mọi truy vấn.',
    invalidJson: 'Nhập một đối tượng JSON hợp lệ.',
  },
  zhHans: {
    legacy: '旧格式', up: '上移', down: '下移',
    automatic: '自动', invalid: 'DNS 配置无效',
    ruleHelp: '在 JSON 中设置动作、目标和匹配条件。没有匹配条件的规则可能匹配所有查询。',
    invalidJson: '请输入有效的 JSON 对象。',
  },
  zhHant: {
    legacy: '舊格式', up: '上移', down: '下移',
    automatic: '自動', invalid: 'DNS 設定無效',
    ruleHelp: '在 JSON 中設定動作、目標及比對條件。沒有比對條件的規則可能符合所有查詢。',
    invalidJson: '請輸入有效的 JSON 物件。',
  },
} satisfies Record<string, Labels>
