/**
 * Shared active domain library state.
 *
 * Both Knowledge.vue (domain picker) and ChatPanel/chatStore (agent filter)
 * read/write this composable so changing the active library in one place
 * automatically updates the other.
 */
import { ref } from 'vue'

const activeLibraryId = ref('')

export function useActiveLibrary() {
  return { activeLibraryId }
}
