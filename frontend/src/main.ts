import { mount } from 'svelte'
import './marlakit'
import './app/style.css'
import Mila from './app/mila.svelte'

// root dom target
const target = document.getElementById('mila')

const app = mount(Mila, {
  target: target ?? document.body
})

export default app
