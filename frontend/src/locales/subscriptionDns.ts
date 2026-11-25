// Client subscription DNS labels. Keep protocol names and native JSON keys unchanged.
const en = {
  final: 'Final DNS', resolver: 'Connection resolver', strategy: 'DNS strategy',
  advanced: 'DNS JSON', legacy: 'Legacy', up: 'Move up', down: 'Move down',
  automatic: 'Automatic', first: 'First server', invalid: 'Invalid DNS configuration',
  removeServer: 'Delete this DNS server?', removeRule: 'Delete this DNS rule?',
  editServer: 'Edit DNS server', editRule: 'Edit DNS rule',
  ruleHelp: 'Set the action, target and match conditions in JSON. Without match conditions the rule may match all queries.',
  invalidJson: 'Enter a valid JSON object.', advancedShape: 'Only dns and optional default_domain_resolver are accepted.',
}
type Labels = Record<keyof typeof en, string>
export const subscriptionDnsMessages = {
  en,
  ru: {
    final: 'Итоговый DNS', resolver: 'Резолвер подключений', strategy: 'Стратегия DNS',
    advanced: 'DNS JSON', legacy: 'Старый формат', up: 'Выше', down: 'Ниже',
    automatic: 'Автоматически', first: 'Первый сервер', invalid: 'Ошибка конфигурации DNS',
    removeServer: 'Удалить этот DNS-сервер?', removeRule: 'Удалить это DNS-правило?',
    editServer: 'Изменить DNS-сервер', editRule: 'Изменить DNS-правило',
    ruleHelp: 'Укажите действие, цель и условия в JSON. Правило без условий может совпадать со всеми запросами.',
    invalidJson: 'Введите корректный объект JSON.', advancedShape: 'Допустимы только dns и необязательный default_domain_resolver.',
  },
  fa: {
    final: 'DNS نهایی', resolver: 'حل‌کنندهٔ اتصال', strategy: 'راهبرد DNS',
    advanced: 'DNS JSON', legacy: 'قالب قدیمی', up: 'انتقال به بالا', down: 'انتقال به پایین',
    automatic: 'خودکار', first: 'سرور نخست', invalid: 'پیکربندی DNS نامعتبر است',
    removeServer: 'این سرور DNS حذف شود؟', removeRule: 'این قانون DNS حذف شود؟',
    editServer: 'ویرایش سرور DNS', editRule: 'ویرایش قانون DNS',
    ruleHelp: 'عمل، مقصد و شرایط تطبیق را در JSON تعیین کنید. قانون بدون شرط ممکن است با همهٔ درخواست‌ها تطبیق داشته باشد.',
    invalidJson: 'یک شیء JSON معتبر وارد کنید.', advancedShape: 'فقط dns و default_domain_resolver اختیاری پذیرفته می‌شوند.',
  },
  vi: {
    final: 'DNS cuối cùng', resolver: 'Bộ phân giải kết nối', strategy: 'Chiến lược DNS',
    advanced: 'DNS JSON', legacy: 'Định dạng cũ', up: 'Di chuyển lên', down: 'Di chuyển xuống',
    automatic: 'Tự động', first: 'Máy chủ đầu tiên', invalid: 'Cấu hình DNS không hợp lệ',
    removeServer: 'Xóa máy chủ DNS này?', removeRule: 'Xóa quy tắc DNS này?',
    editServer: 'Sửa máy chủ DNS', editRule: 'Sửa quy tắc DNS',
    ruleHelp: 'Đặt hành động, đích và điều kiện khớp trong JSON. Quy tắc không có điều kiện có thể khớp mọi truy vấn.',
    invalidJson: 'Nhập một đối tượng JSON hợp lệ.', advancedShape: 'Chỉ chấp nhận dns và default_domain_resolver tùy chọn.',
  },
  zhHans: {
    final: '最终 DNS', resolver: '连接域名解析器', strategy: 'DNS 策略',
    advanced: 'DNS JSON', legacy: '旧格式', up: '上移', down: '下移',
    automatic: '自动', first: '首个服务器', invalid: 'DNS 配置无效',
    removeServer: '删除此 DNS 服务器？', removeRule: '删除此 DNS 规则？',
    editServer: '编辑 DNS 服务器', editRule: '编辑 DNS 规则',
    ruleHelp: '在 JSON 中设置动作、目标和匹配条件。没有匹配条件的规则可能匹配所有查询。',
    invalidJson: '请输入有效的 JSON 对象。', advancedShape: '仅接受 dns 和可选的 default_domain_resolver。',
  },
  zhHant: {
    final: '最終 DNS', resolver: '連線網域解析器', strategy: 'DNS 策略',
    advanced: 'DNS JSON', legacy: '舊格式', up: '上移', down: '下移',
    automatic: '自動', first: '首個伺服器', invalid: 'DNS 設定無效',
    removeServer: '刪除此 DNS 伺服器？', removeRule: '刪除此 DNS 規則？',
    editServer: '編輯 DNS 伺服器', editRule: '編輯 DNS 規則',
    ruleHelp: '在 JSON 中設定動作、目標及比對條件。沒有比對條件的規則可能符合所有查詢。',
    invalidJson: '請輸入有效的 JSON 物件。', advancedShape: '僅接受 dns 與選用的 default_domain_resolver。',
  },
} satisfies Record<string, Labels>
