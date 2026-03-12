<template>

  <a-form-item label="夸克网盘 Cookie">
    <a-space direction="vertical" style="width: 100%">
      <a-textarea v-model="data.cookie" placeholder="请输入夸克网盘的 Cookie" :auto-size="{minRows:3,maxRows:6}" />
      <a-button type="outline" size="small" @click="testCookie" :disabled="!data.cookie">
        测试 Cookie 有效性
      </a-button>
    </a-space>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        登录夸克网盘网页版（建议使用无痕模式），按 F12 打开开发者工具，刷新页面后从 Network 标签的请求中复制 Cookie 字符串。
      </a-typography-text>
    </template>
  </a-form-item>

  <a-divider />

  <a-form-item :label="$t('saveDirectory')">
    <a-space direction="vertical" style="width: 100%">
      <a-input-group compact>
        <a-select
          v-model="data.save_dir"
          placeholder="选择目录或输入新目录名"
          style="width: calc(100% - 100px)"
          allow-clear
          show-search
          :filter-option="filterOption"
          @change="onSaveDirChange"
        >
          <a-option value="">（根目录）</a-option>
          <a-option v-for="dir in directoryList" :key="dir.fid" :value="dir.fid">
            {{ dir.server_filename }}
          </a-option>
        </a-select>
        <a-button type="primary" @click="fetchDirectoryList" :loading="loadingDirectoryList">
          刷新目录
        </a-button>
      </a-input-group>
      
      <a-input
        v-model="newDirectory"
        placeholder="输入新目录名"
        class="input"
      >
        <template #addonAfter>
          <a-button type="link" size="small" @click="createNewDirectory" :disabled="!newDirectory || !data.cookie">
            新建目录
          </a-button>
        </template>
      </a-input>
    </a-space>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        转存文件的保存目录，可从下拉列表选择或输入新目录名。点击"刷新目录"获取夸克网盘根目录列表。
      </a-typography-text>
    </template>
  </a-form-item>

  <a-form-item :label="$t('rateLimit')">
    <a-input-number v-model="data.rate_limit" class="input" :min="1" :max="100" />
    <span class="text-sm text-gray-400 ml-3">次/分钟</span>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        转存速率限制，建议设置为 10 次/分钟以避免触发频率限制
      </a-typography-text>
    </template>
  </a-form-item>

  <a-form-item label="删除广告关键词">
    <a-textarea v-model="data.ad_keywords" placeholder="输入关键词，用逗号分隔" :auto-size="{minRows:2,maxRows:4}" />
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        用于识别需要删除的广告文件的关键词，支持中英文逗号分隔。例如：广告,推广,txt
      </a-typography-text>
    </template>
  </a-form-item>

  <a-form-item label="添加广告地址">
    <a-textarea v-model="data.ad_urls" placeholder="每行一个夸克分享地址" :auto-size="{minRows:3,maxRows:6}" />
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        自定义广告文件的夸克分享地址列表，每行一个URL。转存后会随机选择一个插入到资源目录中。
      </a-typography-text>
    </template>
  </a-form-item>

</template>

<script setup>
import {inject, ref, onMounted} from "vue";
import {Message} from "@arco-design/web-vue";
import axios from "@/api/axios";

const data = inject("options");
const directoryList = ref([]);
const newDirectory = ref("");
const loadingDirectoryList = ref(false);

// 组件挂载时自动加载目录列表（如果已配置 Cookie）
onMounted(async () => {
  if (data.value.cookie) {
    await fetchDirectoryList();
  }
});

// 过滤选项
const filterOption = (input, option) => {
  return option.value.toLowerCase().includes(input.toLowerCase());
};

// 当选择目录变更时，自动保存目录名称
const onSaveDirChange = (value) => {
  if (!value) {
    data.value.save_dir_name = '';
    return;
  }
  
  // 查找对应的目录名称
  const matchedDir = directoryList.value.find(dir => dir.fid === value);
  if (matchedDir) {
    data.value.save_dir_name = matchedDir.server_filename;
  }
};

// 测试 Cookie 有效性
const testCookie = async () => {
  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  try {
    Message.info("正在测试 Cookie 有效性...");
    
    // 调用后端 API
    const response = await axios.post('/plugin/testCookie/QuarkCloudTransfer',
      JSON.stringify({ cookie: data.value.cookie }),
      {
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );
    
    if (response.data && response.data.data) {
      Message.success("Cookie 有效！可以正常使用");
    } else {
      Message.error("Cookie 无效：" + (response.data.message || "未知错误"));
    }
  } catch (error) {
    Message.error("测试 Cookie 失败：" + error.message);
  }
};

// 获取目录列表
const fetchDirectoryList = async () => {
  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  loadingDirectoryList.value = true;
  try {
    Message.info("正在获取目录列表...");
    
    // 调用后端 API
    const response = await axios.post('/plugin/getDirectories/QuarkCloudTransfer', 
      JSON.stringify({ cookie: data.value.cookie }),
      {
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );
    
    if (response.data && response.data.data) {
      directoryList.value = response.data.data || [];
      Message.success(`成功获取 ${directoryList.value.length} 个目录`);
      
      // 如果已有 save_dir 配置，尝试匹配并更新显示名称
      if (data.value.save_dir) {
        const matchedDir = directoryList.value.find(dir => dir.fid === data.value.save_dir);
        if (matchedDir) {
          data.value.save_dir_name = matchedDir.server_filename;
        }
      }
    } else {
      Message.error("获取目录列表失败：" + (response.data.message || "未知错误"));
    }
  } catch (error) {
    Message.error("获取目录列表失败：" + error.message);
  } finally {
    loadingDirectoryList.value = false;
  }
};

// 创建新目录
const createNewDirectory = async () => {
  if (!newDirectory.value.trim()) {
    Message.warning("请输入目录名");
    return;
  }

  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  try {
    // 注意：这里需要后端提供 API 支持
    Message.info(`创建目录: ${newDirectory.value}`);
    
    // 模拟 API 调用
    // 实际实现时需要调用后端 API
    // const response = await axios.post('/admin/api/plugin/quark-cloud-transfer/create-directory', {
    //   cookie: data.value.cookie,
    //   directory: newDirectory.value
    // });
    
    // 成功后
    data.value.save_dir = newDirectory.value;
    data.value.save_dir_name = newDirectory.value; // 保存目录名称
    newDirectory.value = "";
    Message.success("目录创建成功（需要后端 API 支持）");
    
    // 刷新目录列表
    // await fetchDirectoryList();
  } catch (error) {
    Message.error("创建目录失败：" + error.message);
  }
};

// 获取保存目录的显示名称
const getSaveDirDisplayName = () => {
  if (!data.value.save_dir) {
    return '（根目录）';
  }
  
  // 如果已保存目录名称，直接使用
  if (data.value.save_dir_name) {
    return data.value.save_dir_name;
  }
  
  // 否则从目录列表中查找
  const matchedDir = directoryList.value.find(dir => dir.fid === data.value.save_dir);
  if (matchedDir) {
    return matchedDir.server_filename;
  }
  
  // 如果都找不到，显示 ID
  return `目录ID: ${data.value.save_dir}`;
};
</script>

<style scoped>
.input{
  width: 100%;
}
</style>