import menuStyles from "../styles/Menu.module.css"

export function Menu() {
    return <ul className={menuStyles.menu}>
        <li className={menuStyles.menuItem}>Albums</li>
        <li className={menuStyles.menuItem}>Resources</li>
        <li className={menuStyles.menuItem}>Tags</li>
        <li className={menuStyles.menuItem}>People</li>
    </ul>
}