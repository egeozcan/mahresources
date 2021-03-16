import Head from 'next/head'
import styles from '../styles/Layout.module.css'
import {Menu} from "../components/menu";

export default function Home() {
  return (
    <div className={styles.container}>
      <Head>
        <title>Create Next App</title>
        <link rel="icon" href="/favicon.ico" />
      </Head>

      <section className={styles.site}>
        <header className={styles.header}>
          <Menu />
        </header>

        <main className={styles.main}>
          Hollaaaaaaaaa
          <p>tests</p>
        </main>

        <footer className={styles.footer}>
          Footer time
        </footer>
      </section>
    </div>
  )
}
