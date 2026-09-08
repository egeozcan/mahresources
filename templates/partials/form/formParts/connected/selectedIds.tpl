<template x-for="(id, i) in [...$selection.selectedIds]">
    <input type="hidden" name="id" :value="id">
</template>