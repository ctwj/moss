<template>
    <Table modelName="article" :columns="columns" :categoryTree="categoryTreeData" order="id desc" postWidth="98%" postHeight="96%" formLayout="vertical" :postComponent="postComponent" />
</template>

<script setup>
  import {shallowRef} from 'vue'
  import {useRequest} from "vue-request";
  import Table from '@/components/dataTable/index.vue'
  import { searchFilter } from '@/components/dataTable/index.js'
  import Post from './Post.vue'
  import {t} from '@/locale'
  import {categoryTree as fetchCategoryTree} from "@/api/index.js"

  const postComponent = shallowRef(Post);
  
  // 获取分类树数据
  const {data:categoryTreeData} = useRequest(fetchCategoryTree)
  
  const columns = [
    {
      title: t('id'),
      dataIndex: 'id',
      width:100,
      ellipsis:true,
      filterable: searchFilter,
      sortable: { sortDirections: ['ascend', 'descend'] }
    },
    {
      title: t('title'),
      dataIndex: 'title',
      filterable: searchFilter,
      width: 300,
      slotName:'title',
      ellipsis:true,
      tooltip:true,
    },
    {
      title:  t('slug'),
      dataIndex: 'slug',
      filterable: searchFilter,
      width: 140,
      ellipsis:true,
      tooltip:true,
    },

    {
      title: t('category'),
      dataIndex: 'category_id',
      width: 100,
      ellipsis:true,
      filterable: searchFilter,
      slotName:'category',
      align:'right',
    },
    {
      title: t('views'),
      dataIndex: 'views',
      width: 100,
      ellipsis:true,
      sortable: { sortDirections: ['ascend', 'descend'] },
      align:'right',
    },
    {
      title: t('status'),
      dataIndex: 'status',
      slotName:'articleStatus',
      width: 60,
      align:'center',
    },
    {
      title: t('downloadStatus'),
      dataIndex: 'download_paused',
      slotName:'articleDownloadStatus',
      width: 80,
      align:'center',
    },
    {
      title: t('createTime'),
      dataIndex: 'create_time',
      slotName:'time',
      width: 140,
      align:'right',
    },
  ];
</script>