<template x-for="(id, i) in [...$store.bulkSelection.selectedIds]">
    <input type="hidden" name="id" :value="id">
</template>