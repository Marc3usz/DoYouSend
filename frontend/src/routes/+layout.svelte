<script lang="ts">
	let { children } = $props();
</script>

<header>
	<strong>DoYouSend</strong> — komunikaty e-mail i SMS
</header>

<main>
	{@render children()}
</main>

<style>
	header {
		padding: 1rem;
		border-bottom: 1px solid #ddd;
	}
	main {
		padding: 1rem;
		max-width: 60rem;
	}
</style>
