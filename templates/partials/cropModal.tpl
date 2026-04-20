{# Image crop modal. Activated from displayResource.tpl by clicking the Crop button. #}
{# Requires: resource (for ID, Hash, ContentType, Width, Height). #}
<dialog
    id="crop-modal-{{ resource.ID }}"
    class="crop-modal p-0 rounded-lg shadow-xl backdrop:bg-stone-900/60"
    aria-labelledby="crop-modal-title-{{ resource.ID }}"
    x-data="imageCropper({
        resourceId: {{ resource.ID }},
        imageUrl: '/v1/resource/view?id={{ resource.ID }}&v={{ resource.Hash }}',
        initialWidth: {{ resource.Width|default:0 }},
        initialHeight: {{ resource.Height|default:0 }}
    })"
    @close="reset()"
    @cancel="reset()"
>
    <form method="dialog" class="m-0">
        <div class="bg-white w-[90vw] max-w-4xl">
            <header class="px-4 py-3 border-b border-stone-200 flex items-center justify-between">
                <h2 id="crop-modal-title-{{ resource.ID }}" class="text-lg font-semibold text-stone-800">Crop image</h2>
                <button
                    type="button"
                    class="text-stone-500 hover:text-stone-800 p-1"
                    aria-label="Close crop dialog"
                    @click="close()"
                >✕</button>
            </header>

            <div class="px-4 py-3">
                <div class="flex flex-col lg:flex-row gap-4">
                    <div class="flex-1 min-w-0">
                        <div
                            class="crop-stage relative inline-block bg-stone-100 border border-stone-300 select-none"
                            x-ref="stage"
                            @pointerdown.prevent="onPointerDown($event)"
                            @pointermove="onPointerMove($event)"
                            @pointerup="onPointerUp($event)"
                            @pointercancel="onPointerUp($event)"
                        >
                            <img
                                x-ref="image"
                                :src="imageUrl"
                                @load="onImageLoad()"
                                alt="Image being cropped"
                                class="block max-w-full max-h-[60vh] pointer-events-none"
                                draggable="false"
                            >
                            <div
                                class="crop-selection absolute pointer-events-none"
                                x-show="hasSelection()"
                                :style="selectionStyle()"
                                aria-hidden="true"
                            ></div>
                        </div>
                        <p class="text-xs text-stone-500 mt-2">Drag on the image to select the crop area, or type exact pixel values below.</p>
                    </div>

                    <div class="w-full lg:w-64 space-y-3">
                        <div>
                            <label for="crop-aspect-{{ resource.ID }}" class="block text-xs font-medium text-stone-700 mb-1">Aspect ratio</label>
                            <select
                                id="crop-aspect-{{ resource.ID }}"
                                x-model="aspect"
                                @change="applyAspect()"
                                class="w-full rounded-md border-stone-300 text-sm"
                            >
                                <option value="free">Free</option>
                                <option value="1:1">1 : 1 (Square)</option>
                                <option value="16:9">16 : 9</option>
                                <option value="4:3">4 : 3</option>
                                <option value="original">Original</option>
                            </select>
                        </div>

                        <fieldset class="border border-stone-200 rounded-md px-3 py-2">
                            <legend class="text-xs font-medium text-stone-700 px-1">Crop rectangle (image pixels)</legend>
                            <div class="grid grid-cols-2 gap-2 mt-1">
                                <div>
                                    <label for="crop-x-{{ resource.ID }}" class="block text-xs text-stone-600">X</label>
                                    <input id="crop-x-{{ resource.ID }}" type="number" min="0" step="1"
                                        x-model.number="rect.x" @input="clampRect()"
                                        class="w-full rounded-md border-stone-300 text-sm"
                                        aria-describedby="crop-x-hint-{{ resource.ID }}">
                                    <span id="crop-x-hint-{{ resource.ID }}" class="sr-only">Pixels from the left edge of the image</span>
                                </div>
                                <div>
                                    <label for="crop-y-{{ resource.ID }}" class="block text-xs text-stone-600">Y</label>
                                    <input id="crop-y-{{ resource.ID }}" type="number" min="0" step="1"
                                        x-model.number="rect.y" @input="clampRect()"
                                        class="w-full rounded-md border-stone-300 text-sm"
                                        aria-describedby="crop-y-hint-{{ resource.ID }}">
                                    <span id="crop-y-hint-{{ resource.ID }}" class="sr-only">Pixels from the top edge of the image</span>
                                </div>
                                <div>
                                    <label for="crop-w-{{ resource.ID }}" class="block text-xs text-stone-600">Width</label>
                                    <input id="crop-w-{{ resource.ID }}" type="number" min="1" step="1"
                                        x-model.number="rect.width" @input="clampRect()"
                                        class="w-full rounded-md border-stone-300 text-sm">
                                </div>
                                <div>
                                    <label for="crop-h-{{ resource.ID }}" class="block text-xs text-stone-600">Height</label>
                                    <input id="crop-h-{{ resource.ID }}" type="number" min="1" step="1"
                                        x-model.number="rect.height" @input="clampRect()"
                                        class="w-full rounded-md border-stone-300 text-sm">
                                </div>
                            </div>
                        </fieldset>

                        <div>
                            <label for="crop-comment-{{ resource.ID }}" class="block text-xs font-medium text-stone-700 mb-1">Comment (optional)</label>
                            <input id="crop-comment-{{ resource.ID }}" type="text" x-model="comment"
                                class="w-full rounded-md border-stone-300 text-sm"
                                placeholder="e.g. Headshot crop">
                        </div>

                        <p class="text-sm text-stone-700" aria-live="polite">
                            Output: <span x-text="hasSelection() ? (rect.width + ' × ' + rect.height) : '—'"></span>
                        </p>

                        <div role="alert" aria-live="assertive" class="text-sm text-red-700" x-show="errorMessage" x-text="errorMessage"></div>
                    </div>
                </div>
            </div>

            <footer class="px-4 py-3 border-t border-stone-200 flex items-center justify-end gap-2 bg-stone-50">
                <button
                    type="button"
                    @click="close()"
                    class="inline-flex justify-center py-2 px-4 border border-stone-300 bg-white text-sm font-medium font-mono rounded-md text-stone-700 hover:bg-stone-50"
                >Cancel</button>
                <button
                    type="button"
                    @click="submit()"
                    :disabled="!hasSelection() || isSubmitting"
                    class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 disabled:bg-stone-400 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
                >
                    <span x-show="!isSubmitting">Crop</span>
                    <span x-show="isSubmitting">Cropping…</span>
                </button>
            </footer>
        </div>
    </form>
</dialog>
