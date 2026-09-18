import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";
import EmailTemplateEditor from "../EmailTemplateEditor.vue";

const { getEmailTemplates, getEmailTemplate, previewEmailTemplate } = vi.hoisted(() => ({
  getEmailTemplates: vi.fn(),
  getEmailTemplate: vi.fn(),
  previewEmailTemplate: vi.fn(),
}));

vi.mock("@/api", () => ({
  adminAPI: {
    settings: {
      getEmailTemplates,
      getEmailTemplate,
      previewEmailTemplate,
      updateEmailTemplate: vi.fn(),
      restoreOfficialEmailTemplate: vi.fn(),
    },
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}));

vi.mock("vue-i18n", () => ({
  useI18n: () => ({
    locale: { value: "zh-CN" },
    t: (key: string) => key,
  }),
}));

describe("EmailTemplateEditor 工单回复模板", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getEmailTemplates.mockResolvedValue({
      events: [
        { value: "auth.verify_code", label: "验证码", category: "auth" },
        {
          value: "support.ticket_reply",
          label: "工单回复通知",
          category: "support",
        },
      ],
      locales: ["zh", "en"],
      placeholders: [
        "{{ticket_id}}",
        "{{ticket_subject}}",
        "{{reply_content}}",
        "{{reply_time}}",
        "{{ticket_url}}",
        "{{reply_images}}",
      ],
    });
    getEmailTemplate.mockResolvedValue({
      subject: "工单回复",
      html: "<p>{{reply_content}}</p>",
      placeholders: [
        "{{ticket_id}}",
        "{{ticket_subject}}",
        "{{reply_content}}",
        "{{reply_time}}",
        "{{ticket_url}}",
        "{{reply_images}}",
      ],
      is_custom: false,
    });
    previewEmailTemplate.mockResolvedValue({
      subject: "工单回复",
      html: "<p>回复内容</p>",
    });
  });

  it("可选择 support.ticket_reply 并显示工单回复占位符", async () => {
    const wrapper = mount(EmailTemplateEditor);
    await flushPromises();

    const eventSelect = wrapper.get<HTMLSelectElement>("#email-template-event");
    await eventSelect.setValue("support.ticket_reply");
    await flushPromises();

    expect(getEmailTemplate).toHaveBeenLastCalledWith("support.ticket_reply", "zh");
    expect(wrapper.text()).toContain("工单回复通知");
    expect(wrapper.text()).toContain("{{ticket_id}}");
    expect(wrapper.text()).toContain("{{reply_images}}");
  });
});
