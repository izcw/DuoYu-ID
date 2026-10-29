<template>
  <div class="dy-page art-page-view">
    <div class="dy-heading"
      ><div
        ><el-button text class="dy-back-button" @click="$router.push('/manage/users')"
          >← 返回账号管理</el-button
        ><h1>管理权限</h1></div
      ></div
    >
    <section class="dy-panel"
      ><p>系统仅设一位超级管理员，其他账号均为普通用户，不支持分配其他管理角色。</p></section
    >
  </div>
</template>
